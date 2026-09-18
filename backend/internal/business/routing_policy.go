package business

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// RoutingPolicyFingerprint binds an in-memory calculation to the policy that
// produced it, including group bindings and automatic execution switches.
func RoutingPolicyFingerprint(document map[string]any) (string, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
