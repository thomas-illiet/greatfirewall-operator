// Package controller reconciles labelled pods into shared namespace policies.
package controller

import (
	"context"
	"time"

	"github.com/greatfirewall/operator/internal/cache"
	"github.com/greatfirewall/operator/internal/policy"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SnapshotSource retrieves durable profile state, including explicit revocations.
type SnapshotSource interface {
	// Get returns the last usable state and any error from its latest refresh.
	Get(context.Context, string) (cache.Snapshot, error)
}

// Reconciler manages shared policies without changing unrelated namespace rules.
type Reconciler struct {
	client.Client
	Profiles      SnapshotSource
	DNS           policy.DNS
	Interval      time.Duration
	Recorder      events.EventRecorder
	Reconciles    *prometheus.CounterVec
	ProfileStates *prometheus.GaugeVec
}

// Setup maps namespace, pod and policy events to serialized namespace reconciles.
func (r *Reconciler) Setup(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).
		For(&corev1.Namespace{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(namespaceRequest)).
		Watches(&networkingv1.NetworkPolicy{}, handler.EnqueueRequestsFromMapFunc(namespaceRequest)).
		Complete(r)
}

// namespaceRequest routes a namespaced object event to its containing namespace.
func namespaceRequest(_ context.Context, object client.Object) []reconcile.Request {
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{Name: object.GetNamespace()},
	}}
}

// Reconcile repairs the namespace and schedules a periodic refresh even without events.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	err := r.syncNamespace(ctx, request.Name)

	if r.Reconciles != nil {
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		r.Reconciles.WithLabelValues(outcome).Inc()
	}

	return ctrl.Result{RequeueAfter: r.Interval}, err
}

// syncNamespace creates isolation first, then applies active profiles and removes obsolete grants.
func (r *Reconciler) syncNamespace(ctx context.Context, name string) error {
	ns := new(corev1.Namespace)
	if err := r.Get(ctx, client.ObjectKey{Name: name}, ns); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !ns.DeletionTimestamp.IsZero() {
		return nil
	}

	// The baseline must survive failed profile lookups and permission cleanup.
	if err := r.apply(ctx, policy.Baseline(ns)); err != nil {
		return err
	}

	pods := new(corev1.PodList)
	if err := r.List(ctx, pods, client.InNamespace(name)); err != nil {
		return err
	}

	active, unsupported := r.collectProfiles(pods.Items)
	for _, name := range sortedNames(active) {
		if err := r.syncProfile(ctx, ns, name, unsupported[name]); err != nil {
			return err
		}
	}

	return r.cleanPolicies(ctx, ns, pods.Items, active)
}
