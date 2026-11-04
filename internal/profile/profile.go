// Package profile validates complete IPv4-only authorization profiles.
package profile

import (
	"errors"
	"net/netip"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

const MaxRules = 1024

// Port identifies one permitted transport protocol and destination port.
type Port struct {
	Protocol string `json:"protocol"`
	Port     int32  `json:"port"`
}

// Rule associates a destination with only the ports allowed for that destination.
type Rule struct {
	CIDR  string `json:"cidr"`
	Ports []Port `json:"ports"`
}

// Profile is the versioned response returned by the external service.
type Profile struct {
	Profile  string `json:"profile"`
	Revision string `json:"revision"`
	Egress   []Rule `json:"egress"`
}

// ValidName requires a nonempty Kubernetes label value suitable for a profile name.
func ValidName(name string) bool {
	return name != "" && len(validation.IsValidLabelValue(name)) == 0
}

// IPv4Prefix returns a canonical IPv4 prefix and rejects IPv6, including mapped IPv4.
func IPv4Prefix(value string) (string, error) {
	if !strings.Contains(value, "/") {
		ip, err := netip.ParseAddr(value)
		if err != nil || !ip.Is4() {
			return "", errors.New("destination must be IPv4")
		}
		return netip.PrefixFrom(ip, 32).String(), nil
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil || !prefix.Addr().Is4() {
		return "", errors.New("destination must be an IPv4 prefix")
	}
	return prefix.Masked().String(), nil
}

// Validate returns a normalized, independent profile or rejects the entire response.
func Validate(profile Profile, requested string) (Profile, error) {
	validIdentity := ValidName(requested) && profile.Profile == requested
	validRevision := profile.Revision != "" && len(profile.Revision) <= 256
	validRules := profile.Egress != nil && len(profile.Egress) <= MaxRules
	if !validIdentity || !validRevision || !validRules {
		return Profile{}, errors.New("invalid profile identity, revision or egress list")
	}
	out := profile
	out.Egress = make([]Rule, 0, len(profile.Egress))
	for _, rule := range profile.Egress {
		cidr, err := IPv4Prefix(rule.CIDR)
		if err != nil || len(rule.Ports) == 0 || len(rule.Ports) > 128 {
			return Profile{}, errors.New("invalid IPv4 destination or port list")
		}
		ports := append([]Port(nil), rule.Ports...)
		for _, port := range ports {
			if (port.Protocol != "TCP" && port.Protocol != "UDP") || port.Port < 1 || port.Port > 65535 {
				return Profile{}, errors.New("invalid protocol or port")
			}
		}
		sort.Slice(ports, func(i, j int) bool {
			if ports[i].Protocol != ports[j].Protocol {
				return ports[i].Protocol < ports[j].Protocol
			}
			return ports[i].Port < ports[j].Port
		})
		out.Egress = append(out.Egress, Rule{CIDR: cidr, Ports: ports})
	}
	return out, nil
}
