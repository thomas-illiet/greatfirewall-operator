package controller

import (
	"context"
	"log/slog"
	"sort"

	"github.com/greatfirewall/operator/internal/identity"
	"github.com/greatfirewall/operator/internal/policy"
	"github.com/greatfirewall/operator/internal/profile"
	corev1 "k8s.io/api/core/v1"
)

// collectProfiles groups pods and flags unsupported network configurations.
func (r *Reconciler) collectProfiles(pods []corev1.Pod) (map[string]bool, map[string]bool) {
	active := make(map[string]bool)
	unsupported := make(map[string]bool)

	for i := range pods {
		pod := &pods[i]
		name, selected := pod.Labels[identity.ProfileLabel]
		if !selected {
			continue
		}
		if !profile.ValidName(name) {
			r.warn(pod, "InvalidProfile", "Invalid profile label; egress remains denied")
			continue
		}

		active[name] = true
		if err := policy.ValidatePod(pod); err != nil {
			// A shared policy cannot safely grant access to only part of this group.
			unsupported[name] = true
			r.warn(pod, "UnsupportedNetwork", err.Error())
		}
	}

	return active, unsupported
}

// sortedNames makes reconciliation order stable across runs and map iteration orders.
func sortedNames(active map[string]bool) []string {
	names := make([]string, 0, len(active))
	for name := range active {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// syncProfile applies a validated snapshot, retaining deny-all for unknown or revoked profiles.
func (r *Reconciler) syncProfile(
	ctx context.Context,
	ns *corev1.Namespace,
	name string,
	unsupported bool,
) error {
	snapshot, refreshErr := r.Profiles.Get(ctx, name)
	r.reportRefresh(ctx, ns, name, refreshErr)

	allowed := snapshot.Profile
	if snapshot.Revoked || unsupported {
		// Passing nil intentionally removes DNS as well as application grants.
		allowed = nil
	}

	desired, err := policy.Policy(ns, name, allowed, r.DNS)
	if err != nil {
		return err
	}

	return r.apply(ctx, desired)
}

// reportRefresh exposes stale-profile use without logging API bodies or credentials.
func (r *Reconciler) reportRefresh(ctx context.Context, ns *corev1.Namespace, name string, err error) {
	if err != nil {
		r.warn(ns, "ProfileSyncFailed", "Profile refresh failed; retaining the last known state")
		slog.ErrorContext(ctx, "Profile refresh failed", "error", err, "profile", name)
	}
	if r.ProfileStates == nil {
		return
	}

	failed := float64(0)
	if err != nil {
		failed = 1
	}
	r.ProfileStates.WithLabelValues(ns.Name, name).Set(failed)
}
