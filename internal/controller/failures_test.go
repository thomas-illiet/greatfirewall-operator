package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/greatfirewall/operator/internal/identity"
	"github.com/greatfirewall/operator/internal/policy"
	"github.com/greatfirewall/operator/internal/profile"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestUnsafePodsAndForeignPolicies verifies unsupported pods fail closed without adopting foreign rules.
func TestUnsafePodsAndForeignPolicies(t *testing.T) {
	ctx := context.Background()
	r, ns, s := fixture()
	pod := new(corev1.Pod)
	key := client.ObjectKey{Namespace: ns.Name, Name: "one"}
	_ = r.Get(ctx, key, pod)
	pod.Spec.HostNetwork = true
	_ = r.Update(ctx, pod)
	reconcileNamespace(t, r)
	compiled := new(networkingv1.NetworkPolicy)
	_ = r.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: identity.ObjectName("dev")}, compiled)
	if len(compiled.Spec.Egress) != 0 {
		t.Fatal("hostNetwork granted")
	}
	pod.Labels[identity.ProfileLabel] = ""
	_ = r.Update(ctx, pod)
	reconcileNamespace(t, r)
	foreign := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: ns.Name},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{"Egress"},
			Egress:      []networkingv1.NetworkPolicyEgressRule{{}},
		},
	}
	_ = r.Create(ctx, foreign)
	reconcileNamespace(t, r)
	if err := r.Get(ctx, client.ObjectKeyFromObject(foreign), foreign); err != nil {
		t.Fatal("foreign policy deleted")
	}
	s.snapshot.Profile = &profile.Profile{}
	if err := r.syncNamespace(ctx, ns.Name); err == nil {
		t.Fatal("invalid snapshot accepted")
	}
	r.Recorder = nil
	r.warn(ns, "reason", "message")
	if owned(foreign, ns) {
		t.Fatal("foreign owner accepted")
	}
	foreign.Labels = map[string]string{identity.ManagedLabel: identity.ManagedValue}
	if owned(foreign, ns) {
		t.Fatal("missing owner accepted")
	}
}

// TestReconcileFailures checks failures are propagated for retry without allowing unrelated resources.
func TestReconcileFailures(t *testing.T) {
	ctx := context.Background()
	r, ns, _ := fixture()
	if err := r.syncNamespace(ctx, "absent"); err != nil {
		t.Fatal(err)
	}
	foreign := policy.Baseline(ns)
	foreign.Labels = nil
	_ = r.Create(ctx, foreign)
	if err := r.syncNamespace(ctx, ns.Name); err == nil {
		t.Fatal("baseline collision ignored")
	}
	_ = r.Delete(ctx, foreign)
	fault := errors.New("Kubernetes unavailable")
	base := r.Client
	for _, hooks := range []interceptor.Funcs{
		{Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return fault
		}},
		{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error { return fault },
		},
		{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error { return fault },
		},
	} {
		r.Client = interceptor.NewClient(base.(client.WithWatch), hooks)
		if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Name: ns.Name}}); err == nil {
			t.Fatal("API failure swallowed")
		}
	}
	r.Client = base
	if err := r.apply(ctx, policy.Baseline(ns)); err != nil {
		t.Fatal(err)
	}
	r.Client = interceptor.NewClient(base.(client.WithWatch), interceptor.Funcs{
		Get: failPolicyRead,
	})
	if err := r.syncNamespace(ctx, ns.Name); err == nil {
		t.Fatal("policy read error swallowed")
	}
}

// failPolicyRead isolates policy-read failures from successful namespace reads.
func failPolicyRead(
	ctx context.Context,
	kube client.WithWatch,
	key client.ObjectKey,
	object client.Object,
	options ...client.GetOption,
) error {
	if _, ok := object.(*networkingv1.NetworkPolicy); ok {
		return errors.New("policy read unavailable")
	}
	return kube.Get(ctx, key, object, options...)
}

// TestSelectors distinguishes egress overlap from ingress-only or invalid selectors.
func TestSelectors(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{identity.ProfileLabel: "dev"}},
		},
	}
	p := &networkingv1.NetworkPolicy{}
	if selectsManaged(p, pods) {
		t.Fatal("ingress selected")
	}
	p.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
	if !selectsManaged(p, pods) {
		t.Fatal("implicit egress not detected")
	}
	p.Spec.PodSelector.MatchLabels = map[string]string{"other": "value"}
	if selectsManaged(p, pods) {
		t.Fatal("nonmatching policy detected")
	}
	p.Spec.PodSelector.MatchExpressions = []metav1.LabelSelectorRequirement{{Key: "a", Operator: "invalid"}}
	if selectsManaged(p, pods) {
		t.Fatal("invalid selector accepted")
	}
}
