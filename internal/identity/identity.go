// Package identity defines stable names and ownership markers for managed resources.
package identity

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	ProfileLabel      = "greatfirewall/profile"
	ManagedLabel      = "greatfirewall/managed-by"
	ManagedValue      = "greatfirewall-operator"
	ProfileAnnotation = "greatfirewall/profile-name"
	StateAnnotation   = "greatfirewall/state"
	BaselineName      = "greatfirewall-deny-egress"
)

// ObjectName returns a stable resource name; ownership also checks the original profile.
func ObjectName(profile string) string {
	digest := sha256.Sum256([]byte(profile))
	return "gfw-" + hex.EncodeToString(digest[:20])
}
