package controller

import (
	"context"

	"github.com/greatfirewall/operator/internal/identity"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// cleanPolicies deletes unused owned grants and reports overlapping third-party policies.
func (r *Reconciler) cleanPolicies(
	ctx context.Context,
	ns *corev1.Namespace,
	pods []corev1.Pod,
	active map[string]bool,
) error {
	policies := new(networkingv1.NetworkPolicyList)
	if err := r.List(ctx, policies, client.InNamespace(ns.Name)); err != nil {
		return err
	}

	for i := range policies.Items {
		existing := &policies.Items[i]
		if !owned(existing, ns) {
			if selectsManaged(existing, pods) {
				r.warn(ns, "ConcurrentPolicy", "A third-party egress policy selects protected pods")
			}
			continue
		}

		name := existing.Annotations[identity.ProfileAnnotation]
		if existing.Name == identity.BaselineName || active[name] || existing.Name != identity.ObjectName(name) {
			continue
		}

		// Never delete the baseline or a resource whose identity cannot be verified.
		if err := r.Delete(ctx, existing); client.IgnoreNotFound(err) != nil {
			return err
		}
		if r.ProfileStates != nil {
			r.ProfileStates.DeleteLabelValues(ns.Name, name)
		}
	}

	return nil
}
