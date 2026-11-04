package policy

import (
	"testing"

	"github.com/greatfirewall/operator/internal/identity"
	"github.com/greatfirewall/operator/internal/profile"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestPolicy verifies destination-port pairing, targeted DNS and revocation without DNS.
func TestPolicy(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "demo", UID: "ns-1"}}
	base := Baseline(ns)
	if len(base.Spec.PolicyTypes) != 1 || base.Spec.PolicyTypes[0] != "Egress" || base.Spec.PodSelector.MatchExpressions[0].Key != identity.ProfileLabel {
		t.Fatal("incorrect baseline")
	}
	dns := DNS{
		Namespace: "kube-system",
		Selector:  map[string]string{"k8s-app": "kube-dns"},
	}
	p := &profile.Profile{
		Profile:  "dev",
		Revision: "1",
		Egress: []profile.Rule{
			{CIDR: "192.0.2.1", Ports: []profile.Port{{Protocol: "TCP", Port: 443}}},
			{CIDR: "192.0.2.2", Ports: []profile.Port{{Protocol: "UDP", Port: 123}}},
		},
	}
	compiled, err := Policy(ns, "dev", p, dns)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Spec.Egress) != 3 {
		t.Fatal("expected two destination rules and one DNS rule")
	}
	first, second := compiled.Spec.Egress[0], compiled.Spec.Egress[1]
	if first.To[0].IPBlock.CIDR != "192.0.2.1/32" ||
		first.Ports[0].Port.IntVal != 443 || second.Ports[0].Port.IntVal != 123 {
		t.Fatalf("incorrect grants: %+v", compiled.Spec)
	}
	dnsRule := compiled.Spec.Egress[2]
	if len(dnsRule.To) != 1 || dnsRule.To[0].NamespaceSelector == nil || dnsRule.To[0].PodSelector == nil || len(dnsRule.Ports) != 2 {
		t.Fatal("DNS widened")
	}
	compiled, err = Policy(ns, "dev", p, DNS{Addresses: []string{"169.254.20.10"}})
	if err != nil || compiled.Spec.Egress[2].To[0].IPBlock.CIDR != "169.254.20.10/32" {
		t.Fatal("explicit DNS not constrained")
	}
	denied, err := Policy(ns, "dev", nil, DNS{})
	if err != nil || len(denied.Spec.Egress) != 0 || denied.Annotations[identity.StateAnnotation] != "denied" {
		t.Fatal("revocation leaked DNS")
	}
	if _, err := Policy(ns, "other", p, dns); err == nil {
		t.Fatal("wrong profile accepted")
	}
	if _, err := Policy(ns, "dev", p, DNS{}); err == nil {
		t.Fatal("bad DNS accepted")
	}
}

// TestValidateDNS verifies that empty selectors and all IPv6 resolver forms are rejected.
func TestValidateDNS(t *testing.T) {
	for _, dns := range []DNS{
		{},
		{Namespace: "bad/name", Selector: map[string]string{"a": "b"}},
		{Namespace: "dns", Selector: map[string]string{"bad key": "ok"}},
		{Namespace: "dns", Selector: map[string]string{"a": "bad value"}},
		{Addresses: []string{"::1"}},
		{Addresses: []string{"::ffff:192.0.2.1"}},
		{Addresses: []string{"192.0.2.0/24"}},
	} {
		if ValidateDNS(dns) == nil {
			t.Fatalf("invalid DNS accepted: %+v", dns)
		}
	}
}

// TestValidatePod rejects host networking and observed non-IPv4 pod addresses.
func TestValidatePod(t *testing.T) {
	pod := &corev1.Pod{}
	if ValidatePod(pod) != nil {
		t.Fatal("pending pod rejected")
	}
	pod.Status.PodIPs = []corev1.PodIP{{IP: "192.0.2.1"}}
	if ValidatePod(pod) != nil {
		t.Fatal("IPv4 rejected")
	}
	pod.Status.PodIPs = append(pod.Status.PodIPs, corev1.PodIP{IP: "::1"})
	if ValidatePod(pod) == nil {
		t.Fatal("IPv6 accepted")
	}
	pod.Spec.HostNetwork = true
	if ValidatePod(pod) == nil {
		t.Fatal("host network accepted")
	}
}
