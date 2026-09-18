package adminclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const directProbeAnthropicVersion = "2023-06-01"

type ProbePerformanceDescriptor struct {
	Protocol           string
	RequestFingerprint string
}

// PerformanceDescriptor identifies the exact workload without retaining its
// prompt, credentials or endpoint. Different accounts can share a workload.
func (probe *AccountProbe) PerformanceDescriptor(model, prompt string) (ProbePerformanceDescriptor, error) {
	resolved, err := probe.resolveModel(model)
	if err != nil {
		return ProbePerformanceDescriptor{}, err
	}
	_, payload, err := probe.requestPayload(resolved, prompt)
	if err != nil {
		return ProbePerformanceDescriptor{}, err
	}
	apiVersion := ""
	if probe.protocol == "anthropic" {
		apiVersion = directProbeAnthropicVersion
	}
	encoded, err := json.Marshal(struct {
		Version      int    `json:"version"`
		Protocol     string `json:"protocol"`
		APIVersion   string `json:"api_version,omitempty"`
		RequestModel string `json:"request_model"`
		WireModel    string `json:"wire_model"`
		Payload      any    `json:"payload"`
	}{1, probe.protocol, apiVersion, strings.TrimSpace(model), resolved, payload})
	if err != nil {
		return ProbePerformanceDescriptor{}, err
	}
	digest := sha256.Sum256(encoded)
	return ProbePerformanceDescriptor{Protocol: probe.protocol, RequestFingerprint: "v1:" + hex.EncodeToString(digest[:])}, nil
}
