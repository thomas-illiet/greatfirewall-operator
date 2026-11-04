// Package policy compiles validated profiles to standard Kubernetes NetworkPolicies.
package policy

import (
	"github.com/greatfirewall/operator/internal/identity"
	"github.com/greatfirewall/operator/internal/profile"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Metadata identifies a managed policy owned by its containing namespace.
func Metadata(ns *corev1.Namespace, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: ns.Name,
		Labels:    map[string]string{identity.ManagedLabel: identity.ManagedValue},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "v1",
			Kind:       "Namespace",
			Name:       ns.Name,
			UID:        ns.UID,
		}},
	}
}

// Baseline isolates every labelled pod for egress while leaving ingress independent.
func Baseline(ns *corev1.Namespace) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: Metadata(ns, identity.BaselineName),
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      identity.ProfileLabel,
					Operator: metav1.LabelSelectorOpExists,
				}},
			},
		},
	}
}

// Policy compiles a validated profile; nil means deny all, including cluster DNS.
func Policy(
	ns *corev1.Namespace,
	name string,
	document *profile.Profile,
	dns DNS,
) (*networkingv1.NetworkPolicy, error) {
	compiled := deniedProfile(ns, name)
	if document == nil {
		return compiled, nil
	}

	valid, err := profile.Validate(*document, name)
	if err != nil {
		return nil, err
	}
	if err := ValidateDNS(dns); err != nil {
		return nil, err
	}

	// DNS is part of the positive snapshot so revocation also removes this exception.
	compiled.Annotations[identity.StateAnnotation] = "allowed"
	for _, rule := range valid.Egress {
		compiled.Spec.Egress = append(compiled.Spec.Egress, destinationRule(rule))
	}
	compiled.Spec.Egress = append(compiled.Spec.Egress, dnsRule(dns))

	return compiled, nil
}

// deniedProfile builds the stable resource updated in place throughout its lifecycle.
func deniedProfile(ns *corev1.Namespace, name string) *networkingv1.NetworkPolicy {
	compiled := &networkingv1.NetworkPolicy{
		ObjectMeta: Metadata(ns, identity.ObjectName(name)),
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{identity.ProfileLabel: name},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		},
	}
	compiled.Annotations = map[string]string{
		identity.ProfileAnnotation: name,
		identity.StateAnnotation:   "denied",
	}
	return compiled
}

// destinationRule keeps each address paired with its own ports, avoiding a cross-product.
func destinationRule(rule profile.Rule) networkingv1.NetworkPolicyEgressRule {
	entry := networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{{
			IPBlock: &networkingv1.IPBlock{CIDR: rule.CIDR},
		}},
	}
	for _, port := range rule.Ports {
		entry.Ports = append(entry.Ports, networkPort(corev1.Protocol(port.Protocol), port.Port))
	}
	return entry
}

// networkPort converts a validated numeric transport port to the Kubernetes representation.
func networkPort(protocol corev1.Protocol, port int32) networkingv1.NetworkPolicyPort {
	value := intstr.FromInt32(port)
	return networkingv1.NetworkPolicyPort{Protocol: &protocol, Port: &value}
}
