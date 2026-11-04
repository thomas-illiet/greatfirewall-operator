package profile

import (
	"strings"
	"testing"
)

// fixture returns a fresh profile with independently scoped ports.
func fixture() Profile {
	return Profile{
		Profile:  "sandbox",
		Revision: "1",
		Egress: []Rule{
			{
				CIDR: "192.0.2.19",
				Ports: []Port{
					{Protocol: "UDP", Port: 53},
					{Protocol: "TCP", Port: 443},
					{Protocol: "TCP", Port: 80},
				},
			},
		},
	}
}

// TestValidate verifies strict IPv4 validation and rejects incomplete or overly broad grants.
func TestValidate(t *testing.T) {
	valid, err := Validate(fixture(), "sandbox")
	if err != nil || valid.Egress[0].CIDR != "192.0.2.19/32" || valid.Egress[0].Ports[0].Port != 80 {
		t.Fatalf("unexpected normalized profile: %+v %v", valid, err)
	}
	cases := []func(*Profile){
		func(p *Profile) { p.Profile = "other" },
		func(p *Profile) { p.Revision = "" },
		func(p *Profile) { p.Revision = strings.Repeat("x", 257) },
		func(p *Profile) { p.Egress = nil },
		func(p *Profile) { p.Egress = make([]Rule, MaxRules+1) },
		func(p *Profile) { p.Egress[0].CIDR = "::1" },
		func(p *Profile) { p.Egress[0].CIDR = "::ffff:192.0.2.1" },
		func(p *Profile) { p.Egress[0].CIDR = "2001:db8::/64" },
		func(p *Profile) { p.Egress[0].CIDR = "::ffff:192.0.2.1/128" },
		func(p *Profile) { p.Egress[0].Ports = nil },
		func(p *Profile) { p.Egress[0].Ports = make([]Port, 129) },
		func(p *Profile) { p.Egress[0].Ports[0].Protocol = "SCTP" },
		func(p *Profile) { p.Egress[0].Ports[0].Port = 0 },
		func(p *Profile) { p.Egress[0].Ports[0].Port = 65536 },
	}
	for i, change := range cases {
		p := fixture()
		change(&p)
		if _, err := Validate(p, "sandbox"); err == nil {
			t.Errorf("case %d accepted invalid profile", i)
		}
	}
	if _, err := Validate(fixture(), ""); err == nil {
		t.Fatal("empty name accepted")
	}
	p := fixture()
	p.Egress = []Rule{}
	if _, err := Validate(p, "sandbox"); err != nil {
		t.Fatal(err)
	}
}

// TestIPv4Prefix verifies canonical prefixes and rejects IPv6 and malformed addresses.
func TestIPv4Prefix(t *testing.T) {
	for input, want := range map[string]string{
		"192.0.2.3":     "192.0.2.3/32",
		"192.0.2.37/24": "192.0.2.0/24",
		"0.0.0.0/0":     "0.0.0.0/0",
	} {
		got, err := IPv4Prefix(input)
		if err != nil || got != want {
			t.Fatalf("%s: %s %v", input, got, err)
		}
	}
	for _, input := range []string{"bad", "192.0.2.1/33", "::1", "::/0", "::ffff:192.0.2.1/128"} {
		if _, err := IPv4Prefix(input); err == nil {
			t.Fatal(input)
		}
	}
	if ValidName("") || ValidName("a/b") || !ValidName("Team_dev.1") {
		t.Fatal("incorrect name validation")
	}
}
