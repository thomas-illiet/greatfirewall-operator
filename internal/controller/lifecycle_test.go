package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/greatfirewall/operator/internal/cache"
	"github.com/greatfirewall/operator/internal/identity"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestSharedPolicies verifies pods using the same profile share one authorization policy.
func TestSharedPolicies(t *testing.T) {
	ctx := context.Background()
	r, ns, _ := fixture()
	reconcileNamespace(t, r)

	policies := new(networkingv1.NetworkPolicyList)
	if err := r.List(ctx, policies, client.InNamespace(ns.Name)); err != nil {
		t.Fatal(err)
	}
	if len(policies.Items) != 2 {
		t.Fatal("did not share policy")
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name}}
	requests := namespaceRequest(ctx, pod)
	if len(requests) != 1 || requests[0].Name != ns.Name {
		t.Fatal("event incorrectly mapped")
	}
}

// TestPolicyRepair verifies rule drift and accidental deletion are repaired.
func TestPolicyRepair(t *testing.T) {
	ctx := context.Background()
	r, ns, _ := fixture()
	reconcileNamespace(t, r)

	key := client.ObjectKey{Namespace: ns.Name, Name: identity.ObjectName("dev")}
	compiled := readPolicy(t, r, key)
	compiled.Spec.Egress = nil
	compiled.Annotations = nil
	updateObject(t, r, compiled)

	// Ownership identity is required; repairing the profile annotation deliberately requires an administrator.
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Name: ns.Name}}); err == nil {
		t.Fatal("annotation ownership conflict ignored")
	}
	compiled = readPolicy(t, r, key)
	compiled.Annotations = map[string]string{identity.ProfileAnnotation: "dev"}
	updateObject(t, r, compiled)

	reconcileNamespace(t, r)
	compiled = readPolicy(t, r, key)
	if len(compiled.Spec.Egress) != 2 {
		t.Fatal("drift not repaired")
	}

	if err := r.Delete(ctx, compiled); err != nil {
		t.Fatal(err)
	}
	reconcileNamespace(t, r)
	if len(readPolicy(t, r, key).Spec.Egress) != 2 {
		t.Fatal("deleted policy not restored")
	}
}

// TestProfileOutageAndRevocation distinguishes retained state from explicit revocation.
func TestProfileOutageAndRevocation(t *testing.T) {
	r, ns, s := fixture()
	reconcileNamespace(t, r)
	key := client.ObjectKey{Namespace: ns.Name, Name: identity.ObjectName("dev")}

	s.err = errors.New("offline")
	reconcileNamespace(t, r)
	if len(readPolicy(t, r, key).Spec.Egress) != 2 {
		t.Fatal("outage lost permissions")
	}

	s.snapshot = cache.Snapshot{Name: "dev", Revoked: true}
	s.err = nil
	reconcileNamespace(t, r)
	if len(readPolicy(t, r, key).Spec.Egress) != 0 {
		t.Fatal("revocation retained DNS")
	}
}

// TestLabelRemoval removes unused grants while retaining the namespace baseline.
func TestLabelRemoval(t *testing.T) {
	ctx := context.Background()
	r, ns, _ := fixture()
	reconcileNamespace(t, r)

	for _, name := range []string{"one", "two"} {
		pod := new(corev1.Pod)
		if err := r.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: name}, pod); err != nil {
			t.Fatal(err)
		}
		pod.Labels = nil
		updateObject(t, r, pod)
	}
	reconcileNamespace(t, r)

	key := client.ObjectKey{Namespace: ns.Name, Name: identity.ObjectName("dev")}
	if err := r.Get(ctx, key, new(networkingv1.NetworkPolicy)); err == nil {
		t.Fatal("unused policy retained")
	}
	policies := new(networkingv1.NetworkPolicyList)
	if err := r.List(ctx, policies, client.InNamespace(ns.Name)); err != nil {
		t.Fatal(err)
	}
	if len(policies.Items) != 1 || policies.Items[0].Name != identity.BaselineName {
		t.Fatal("baseline removed")
	}
}
