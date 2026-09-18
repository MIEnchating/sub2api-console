package routing

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type probePerformanceKey struct {
	requestModel, actualModel, protocol, prompt, request string
}

type probePerformanceSummary struct {
	p95MS   float64
	samples int
}

// Probe performance is independent of traffic percentiles. Only a current,
// uninterrupted series of identical requests can supply comparison evidence.
func probePerformanceEvidence(rows []business.RoutingSample, policy map[string]any, account business.RoutingAccount, config engineConfig, now time.Time) map[probePerformanceKey]probePerformanceSummary {
	if !config.probePerformanceEnabled {
		return nil
	}
	probePolicy, _ := policy["probe"].(map[string]any)
	prompt, _ := probePolicy["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "hi"
	}
	expectedPrompt := business.ProbePromptFingerprint(prompt)
	probes := freshSourceSamples(rows, "active-probe", now, config.historyMaxAge, len(rows))
	modelWindow := max(config.longWindow, config.performanceMinSamples)
	keys := map[string]probePerformanceKey{}
	lastObserved := map[string]time.Time{}
	blocked := map[string]bool{}
	seen := map[string]bool{}
	values := map[probePerformanceKey][]float64{}
	for _, row := range probes {
		key := probeComparisonKey(row.Payload)
		if blocked[key.requestModel] {
			continue
		}
		if len(values[key]) >= modelWindow {
			continue
		}
		observed, _ := time.Parse(time.RFC3339Nano, row.ObservedAt)
		previous, started := keys[key.requestModel]
		valid := validPerformanceProbe(row) && key.prompt == expectedPrompt && probeModelStillConfigured(key.requestModel, policy, account)
		interrupted := started && lastObserved[key.requestModel].Sub(observed) > max(config.probeInterval, config.probeMaxAge)
		if !valid || (started && previous != key) || interrupted || (!started && now.Sub(observed) > config.probeMaxAge) {
			blocked[key.requestModel] = true
			continue
		}
		keys[key.requestModel] = key
		lastObserved[key.requestModel] = observed
		identity := row.ObservedAt + "|" + key.requestModel
		if seen[identity] {
			continue
		}
		seen[identity] = true
		latency := measuredFirstTokenMS(Sample{LatencyP95: row.LatencyP95, Payload: row.Payload})
		values[key] = append(values[key], *latency)
	}
	result := map[probePerformanceKey]probePerformanceSummary{}
	for key, latencies := range values {
		sort.Float64s(latencies)
		result[key] = probePerformanceSummary{samples: len(latencies), p95MS: latencies[max(0, int(math.Ceil(.95*float64(len(latencies))))-1)]}
	}
	return result
}

func probeComparisonKey(payload map[string]any) probePerformanceKey {
	return probePerformanceKey{
		requestModel: payloadText(payload, "request_model"), actualModel: payloadText(payload, "actual_model"),
		protocol: payloadText(payload, "probe_protocol"), prompt: payloadText(payload, "probe_prompt_fingerprint"),
		request: payloadText(payload, "probe_request_fingerprint"),
	}
}

func validPerformanceProbe(row business.RoutingSample) bool {
	key := probeComparisonKey(row.Payload)
	if !successfulRoutingSample(row) || row.Payload["performance_eligible"] != true || row.Payload["measured_first_token"] != true || row.Payload["retry_recovered"] == true {
		return false
	}
	if status := routingSampleStatus(row); status != nil && (*status < 200 || *status >= 300) {
		return false
	}
	if statuses, ok := row.Payload["attempt_status_codes"].([]any); ok && len(statuses) > 1 {
		return false
	}
	if key.requestModel == "" || key.actualModel == "" || key.protocol == "" || key.request == "" || row.Payload["latency_source"] != "upstream_direct.first_content" {
		return false
	}
	return measuredFirstTokenMS(Sample{LatencyP95: row.LatencyP95, Payload: row.Payload}) != nil
}

func probeModelStillConfigured(model string, policy map[string]any, account business.RoutingAccount) bool {
	models, _ := policy["account_test_models"].(map[string]any)
	if configured, found := models[account.ID]; found {
		switch values := configured.(type) {
		case string:
			return model == strings.TrimSpace(values)
		case []any:
			for _, value := range values {
				if value, ok := value.(string); ok && model == strings.TrimSpace(value) {
					return true
				}
			}
		}
		return false
	}
	bindings, _ := policy["group_policy_bindings"].(map[string]any)
	if account.GroupID != nil {
		binding, _ := bindings[*account.GroupID].(map[string]any)
		if configured := payloadText(binding, "probe_model"); configured != "" {
			return model == configured
		}
	}
	probe, _ := policy["probe"].(map[string]any)
	configured := payloadText(probe, "model")
	return configured == "" || model == configured
}

func payloadText(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func placementEvidenceReady(item *candidate, config engineConfig) bool {
	minimum := max(1, config.performanceMinSamples)
	seen := map[string]bool{}
	for _, row := range item.rows {
		if !successfulRoutingSample(row) {
			continue
		}
		classified := classify(Sample{Result: row.Result, FailureReason: row.FailureReason, Source: row.Source, LatencyP95: row.LatencyP95, StatusCode: routingSampleStatus(row), Payload: row.Payload}, config.sampleClassification)
		if classified.Neutral || classified.Failure || classified.Fatal {
			continue
		}
		identity := row.EvidenceKey
		if identity == "" {
			identity = payloadText(row.Payload, "request_id")
		}
		if identity == "" {
			identity = fmt.Sprintf("%s|%s|%s", row.Source, row.ObservedAt, payloadText(row.Payload, "request_model"))
		}
		seen[row.Source+"|"+identity] = true
	}
	if len(seen) >= minimum {
		return true
	}
	if config.probePerformanceEnabled {
		for _, series := range item.probePerformance {
			if series.samples >= minimum {
				return true
			}
		}
	}
	return false
}

func comparablePlacementLatencies(challenger, incumbent *candidate, config engineConfig) (float64, float64, bool) {
	minimum := max(1, config.performanceMinSamples)
	if challenger.performanceSamples >= minimum && incumbent.performanceSamples >= minimum && challenger.performanceModel != "" && challenger.performanceModel == incumbent.performanceModel && challenger.performanceP95MS != nil && incumbent.performanceP95MS != nil {
		return cappedPlacementLatencies(*challenger.performanceP95MS, *incumbent.performanceP95MS, config.speedAdvantageCap)
	}
	if !config.probePerformanceEnabled {
		return 1000, 1000, false
	}
	// Use the least favorable ratio across shared models, so a single easy
	// model cannot hide a regression in another measured model.
	ratio, comparable := 0.0, false
	for key, a := range challenger.probePerformance {
		b, ok := incumbent.probePerformance[key]
		if !ok || a.samples < minimum || b.samples < minimum {
			continue
		}
		ratio = max(ratio, a.p95MS/b.p95MS)
		comparable = true
	}
	if !comparable {
		return 1000, 1000, false
	}
	return cappedPlacementLatencies(ratio*1000, 1000, min(2, config.speedAdvantageCap))
}

func cappedPlacementLatencies(a, b, cap float64) (float64, float64, bool) {
	if a <= 0 || b <= 0 || math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return 1000, 1000, false
	}
	cap = max(1, cap)
	fastest := min(a, b)
	return min(a, fastest*cap), min(b, fastest*cap), true
}
