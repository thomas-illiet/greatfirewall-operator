package policy

import (
	"errors"
	"net/netip"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// DNS describes trusted cluster resolvers; addresses select explicit IPv4 resolvers.
type DNS struct {
	Namespace string            `json:"namespace"`
	Selector  map[string]string `json:"selector"`
	Addresses []string          `json:"addresses"`
}

// ValidateDNS validates resolver configuration without allowing IPv6 or empty selectors.
func ValidateDNS(dns DNS) error {
	if len(dns.Addresses) > 0 {
		for _, value := range dns.Addresses {
			ip, err := netip.ParseAddr(value)
			if err != nil || !ip.Is4() {
				return errors.New("DNS addresses must be individual IPv4 addresses")
			}
		}
		return nil
	}
	if dns.Namespace == "" || len(validation.IsDNS1123Label(dns.Namespace)) != 0 || len(dns.Selector) == 0 {
		return errors.New("DNS namespace and nonempty selector required")
	}
	for key, value := range dns.Selector {
		if len(validation.IsQualifiedName(key)) != 0 || len(validation.IsValidLabelValue(value)) != 0 {
			return errors.New("invalid DNS selector")
		}
	}
	return nil
}

// dnsRule restricts DNS to explicit resolver addresses or pods in one trusted namespace.
func dnsRule(dns DNS) networkingv1.NetworkPolicyEgressRule {
	rule := networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			networkPort(corev1.ProtocolTCP, 53),
			networkPort(corev1.ProtocolUDP, 53),
		},
	}

	if len(dns.Addresses) > 0 {
		for _, address := range dns.Addresses {
			rule.To = append(rule.To, networkingv1.NetworkPolicyPeer{
				IPBlock: &networkingv1.IPBlock{CIDR: address + "/32"},
			})
		}
		return rule
	}

	// Both selectors belong to the same peer: namespace AND pod, never OR.
	rule.To = []networkingv1.NetworkPolicyPeer{{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": dns.Namespace},
		},
		PodSelector: &metav1.LabelSelector{MatchLabels: dns.Selector},
	}}
	return rule
}
