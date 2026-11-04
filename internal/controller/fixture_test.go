package controller

import (
	"context"
	"testing"
	"time"

	"github.com/greatfirewall/operator/internal/cache"
	"github.com/greatfirewall/operator/internal/identity"
	"github.com/greatfirewall/operator/internal/policy"
	"github.com/greatfirewall/operator/internal/profile"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// source supplies mutable profile snapshots to reconciliation tests.
type source struct {
	snapshot cache.Snapshot
	err      error
}

// Get returns test profile state without network dependencies.
func (s *source) Get(context.Context, string) (cache.Snapshot, error) {
	return s.snapshot, s.err
}

// fixture builds a namespace and two labelled pods sharing one valid profile.
func fixture() (*Reconciler, *corev1.Namespace, *source) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "demo", UID: "uid"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "one",
			Namespace: ns.Name,
			Labels:    map[string]string{identity.ProfileLabel: "dev"},
		},
	}
	two := pod.DeepCopy()
	two.Name = "two"
	s := &source{
		snapshot: cache.Snapshot{
			Name: "dev",
			Profile: &profile.Profile{
				Profile:  "dev",
				Revision: "1",
				Egress:   []profile.Rule{{CIDR: "192.0.2.1", Ports: []profile.Port{{Protocol: "TCP", Port: 443}}}},
			},
		},
	}
	r := &Reconciler{
		Client:        fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, pod, two).Build(),
		Profiles:      s,
		DNS:           policy.DNS{Addresses: []string{"10.96.0.10"}},
		Interval:      time.Minute,
		Recorder:      events.NewFakeRecorder(100),
		Reconciles:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_reconciles", Help: "tests"}, []string{"outcome"}),
		ProfileStates: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_states", Help: "tests"}, []string{"namespace", "profile"}),
	}
	return r, ns, s
}

// reconcileNamespace runs the namespace controller and fails the test on an unexpected API error.
func reconcileNamespace(t *testing.T, r *Reconciler) {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "demo"}})
	if err != nil || result.RequeueAfter != time.Minute {
		t.Fatalf("reconcile: %v", err)
	}
}

// readPolicy loads one policy and stops immediately if test setup is inconsistent.
func readPolicy(t *testing.T, r *Reconciler, key client.ObjectKey) *networkingv1.NetworkPolicy {
	t.Helper()
	compiled := new(networkingv1.NetworkPolicy)
	if err := r.Get(context.Background(), key, compiled); err != nil {
		t.Fatal(err)
	}
	return compiled
}

// updateObject persists a fixture mutation before exercising reconciliation.
func updateObject(t *testing.T, r *Reconciler, object client.Object) {
	t.Helper()
	if err := r.Update(context.Background(), object); err != nil {
		t.Fatal(err)
	}
}
