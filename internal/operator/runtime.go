// Package operator wires Kubernetes informers, controllers, metrics and leader election.
package operator

import (
	"context"
	"errors"

	"github.com/greatfirewall/operator/internal/api"
	"github.com/greatfirewall/operator/internal/cache"
	"github.com/greatfirewall/operator/internal/config"
	"github.com/greatfirewall/operator/internal/controller"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// Start installs the controller and runs its elected manager until context cancellation.
func Start(ctx context.Context, settings config.Operator) error {
	remote, err := api.NewAPIClient(settings.APIURL, settings.TokenFile, settings.CAFile, settings.HTTPTimeout)
	if err != nil {
		return err
	}
	kubeconfig, err := ctrl.GetConfig()
	if err != nil {
		return errors.New("cannot load Kubernetes configuration")
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	options := ctrl.Options{
		Scheme:                  scheme,
		HealthProbeBindAddress:  settings.HealthAddress,
		LeaderElection:          settings.LeaderElection,
		LeaderElectionID:        "greatfirewall-operator",
		LeaderElectionNamespace: settings.Namespace,
		Metrics: metricsserver.Options{
			BindAddress:    settings.MetricsAddress,
			SecureServing:  true,
			FilterProvider: filters.WithAuthenticationAndAuthorization,
		},
		Client: client.Options{
			// Cache snapshots are read directly, without a cluster-wide ConfigMap informer.
			Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.ConfigMap{}}},
		},
	}
	manager, err := ctrl.NewManager(kubeconfig, options)
	if err != nil {
		return err
	}
	return Run(ctx, manager, settings, remote)
}

// Run registers application components on an existing manager, enabling integration testing.
func Run(ctx context.Context, manager ctrl.Manager, settings config.Operator, remote api.Fetcher) error {
	store := &cache.Store{
		Client: manager.GetClient(), Namespace: settings.Namespace,
		Fetcher: remote, Interval: settings.RefreshInterval,
	}
	reconciles := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "greatfirewall_reconciliations_total",
		Help: "Namespace reconciliation outcomes.",
	}, []string{"outcome"})
	states := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "greatfirewall_profile_sync_error",
		Help: "One when the latest refresh failed.",
	}, []string{"namespace", "profile"})
	if err := metrics.Registry.Register(reconciles); err != nil {
		return err
	}
	defer metrics.Registry.Unregister(reconciles)
	if err := metrics.Registry.Register(states); err != nil {
		return err
	}
	defer metrics.Registry.Unregister(states)
	reconciler := &controller.Reconciler{
		Client:        manager.GetClient(),
		Profiles:      store,
		DNS:           settings.DNS,
		Interval:      settings.RefreshInterval,
		Recorder:      manager.GetEventRecorder("greatfirewall"),
		Reconciles:    reconciles,
		ProfileStates: states,
	}
	if err := reconciler.Setup(manager); err != nil {
		return err
	}
	if err := manager.AddHealthzCheck("health", healthz.Ping); err != nil {
		return err
	}
	if err := manager.AddReadyzCheck("ready", healthz.Ping); err != nil {
		return err
	}
	return manager.Start(ctx)
}
