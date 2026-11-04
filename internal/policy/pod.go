package policy

import (
	"errors"
	"fmt"
	"net/netip"

	corev1 "k8s.io/api/core/v1"
)

// ValidatePod rejects observed networking modes outside the IPv4-only platform contract.
func ValidatePod(pod *corev1.Pod) error {
	if pod.Spec.HostNetwork {
		return errors.New("hostNetwork is unsupported")
	}
	for _, address := range pod.Status.PodIPs {
		ip, err := netip.ParseAddr(address.IP)
		if err != nil || !ip.Is4() {
			return fmt.Errorf("pod %s has a non-IPv4 address", pod.Name)
		}
	}
	return nil
}
