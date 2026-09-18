package business

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
)

// ProbePromptFingerprint uses the exact prompt after the caller's existing
// trim/default handling. Internal whitespace and case affect the workload.
func ProbePromptFingerprint(prompt string) string {
	digest := sha256.Sum256([]byte(prompt))
	return "v1:" + hex.EncodeToString(digest[:])
}

func probePerformanceEligible(sample ProbeSample) bool {
	if sample.Result != "通过" || sample.Attempts != 1 || sample.RetryRecovered || !sample.MeasuredFirstToken || sample.LatencyP95 == nil {
		return false
	}
	if sample.FailureReason != nil && strings.TrimSpace(*sample.FailureReason) != "" {
		return false
	}
	if strings.TrimSpace(sample.RequestModel) == "" || strings.TrimSpace(sample.ActualModel) == "" || !validProbeFingerprint(sample.ProbePromptFingerprint) || !validProbeFingerprint(sample.ProbeRequestFingerprint) {
		return false
	}
	switch sample.ProbeProtocol {
	case "responses", "chat_completions", "anthropic", "gemini":
	default:
		return false
	}
	latency, err := strconv.ParseFloat(*sample.LatencyP95, 64)
	return err == nil && !math.IsNaN(latency) && !math.IsInf(latency, 0) && latency > 0
}

func validProbeFingerprint(value string) bool {
	if !strings.HasPrefix(value, "v1:") || len(value) != 67 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "v1:"))
	return err == nil
}
