package routing

import (
	"context"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestGuardianPolicyPersistsAndReachesRoutingExecution(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "policy-contract.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdatePolicy(ctx, map[string]any{
		"global_strategy":       "speed_first",
		"missing_rate_fallback": "fail_closed",
		"change_threshold":      "0.23",
		"cooldown_seconds":      int64(73),
		"auto_apply": map[string]any{
			"schedulable": false, "priority": true, "load_factor": false, "concurrency": true,
		},
		"excluded_group_ids":       []any{"8"},
		"traffic_enabled":          false,
		"probe_interval_seconds":   int64(311),
		"probe_model":              "probe-contract-model",
		"traffic_lookback_minutes": int64(131),
		"max_samples_per_account":  int64(67),
		"advanced_policy": map[string]any{
			"weights": map[string]any{
				"enabled": false, "budget": int64(777), "gate_floor": 41.5,
				"price_exp": 1.7, "speed_exp": 2.3, "balanced_price_ratio": 0.35,
				"min_load_factor": int64(2), "max_load_factor": int64(97),
			},
			"scope": map[string]any{
				"managed_group_mode": "selected", "managed_group_ids": []any{"7"},
				"account_types": []any{"oauth"}, "platforms": []any{"openai"},
				"paused_account_ids":   []any{"41"},
				"excluded_account_ids": []any{"42"}, "manual_fused_account_ids": []any{"43"},
			},
			"probe": map[string]any{
				"enabled": false, "timeout_seconds": int64(61), "concurrency": int64(5),
				"prompt": "contract probe", "skip_when_traffic_fresh": false, "traffic_fresh_seconds": int64(181),
			},
			"traffic": map[string]any{"refresh_seconds": int64(71)},
			"scoring": map[string]any{
				"short_window": int64(11), "long_window": int64(67), "latest_weight": 0.55,
				"short_ratio": 0.65, "slow_ttfb_ms": int64(5100),
				"event_scores": map[string]any{
					"perfect": 99.5, "slow_ttfb": 64.5, "upstream_unknown": 39.5,
					"gateway_error": 24.5, "quota_exhausted": 14.5, "probe_fail": 9.5, "fatal": 0.0,
				},
			},
			"breaker": map[string]any{
				"enabled": false, "hard_fatal": false, "http_window": int64(7), "http_failures": int64(4),
				"http_score_below": 61.5, "latency_window": int64(12), "latency_occurrences": int64(6),
				"latency_ttfb_ms": int64(16000), "max_switch_per_round": int64(2),
				"min_pool_size": int64(2), "min_pool_score": 4.5, "fused_cooldown_seconds": int64(181),
				"instant_status_codes": []any{int64(401), int64(403)}, "http_degrade_only": false, "latency_degrade_only": false,
			},
			"degrade": map[string]any{
				"enabled": false, "score_threshold": 76.5, "priority_step": int64(11),
				"load_factor_ratio": 0.45, "min_load_factor": int64(2),
			},
			"recovery": map[string]any{
				"enabled": false, "probe_interval_seconds": int64(181), "target_score": 76.5,
				"success_count": int64(3), "hold_seconds": int64(61),
			},
			"scaling": map[string]any{
				"enabled": true, "global_max_concurrency": int64(901), "min_per_account": int64(4),
				"max_per_account": int64(251), "scale_up_ratio": 0.81, "step_up": int64(6),
				"step_down": int64(7), "cooldown_seconds": int64(62),
			},
			"cleanup": map[string]any{
				"enabled": true, "action": "disable", "occurrences": int64(4), "window": int64(6),
				"min_fused_minutes": int64(31), "max_per_round": int64(2), "keep_last_in_group": false,
				"only_auth_errors": false, "trigger_status_codes": []any{int64(401), int64(403)},
			},
			"classify": map[string]any{
				"fatal_patterns":       []any{"invalid contract key"},
				"gateway_status_codes": []any{int64(429), int64(502)},
			},
		},
	}, "contract-test")
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.ControlPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config, err := parseEngineConfig(document)
	if err != nil {
		t.Fatal(err)
	}

	assertEngineContract(t, config)
	health, err := HealthScore([]Sample{{Result: "通过"}}, document)
	if err != nil {
		t.Fatal(err)
	}
	if health.HealthScore != 99.5 || health.ShortScore != 99.5 || health.LongScore != 99.5 {
		t.Fatalf("event score did not reach scoring execution: %#v", health)
	}
}

func assertEngineContract(t *testing.T, config engineConfig) {
	t.Helper()
	if config.strategy != "speed_first" || config.trafficEnabled ||
		config.shortWindow != 11 || config.longWindow != 67 || config.breakerEnabled || config.hardFatal ||
		config.httpWindow != 7 || config.httpFailures != 4 || config.httpScoreBelow != 61.5 ||
		config.latencyWindow != 12 || config.latencyOccurrences != 6 || config.latencyTTFBMS != 16000 ||
		config.maxSwitch != 2 || config.minPool != 2 || config.minPoolScore != 4.5 ||
		config.fusedCooldown != 181*time.Second || config.httpDegradeOnly || config.latencyDegradeOnly ||
		config.degradeEnabled || config.degradeThreshold != 76.5 || config.degradePriorityStep != 11 ||
		config.degradeLoadRatio != 0.45 || config.degradeMinLoad != 2 || config.recoveryEnabled ||
		config.recoveryTarget != 76.5 || config.recoverySuccesses != 3 || config.recoveryHold != 61*time.Second ||
		config.weightsEnabled || config.weightBudget != 777 || config.gateFloor != 41.5 || config.priceExp != 1.7 ||
		config.speedExp != 2.3 || config.balancedPriceRatio != 0.35 || config.missingRateFallback != "fail_closed" ||
		config.cooldown != 73*time.Second || config.minLoadFactor != 2 || config.maxLoadFactor != 97 ||
		!config.scalingEnabled || config.scalingGlobalMax != 901 || config.scalingMin != 4 || config.scalingMax != 251 ||
		config.scalingUpRatio != 0.81 || config.scalingStepUp != 6 || config.scalingStepDown != 7 ||
		config.scalingCooldown != 62*time.Second || config.managedMode != "selected" ||
		!config.cleanupEnabled || config.cleanupAction != "disable" || config.cleanupOccurrences != 4 ||
		config.cleanupWindow != 6 || config.cleanupObservation != 31*time.Minute || config.cleanupMaxPerRound != 2 ||
		config.cleanupKeepLast || config.cleanupOnlyAuth {
		t.Fatalf("persisted policy did not reach routing execution: %#v", config)
	}
	if config.changeThreshold.Cmp(big.NewRat(23, 100)) != 0 {
		t.Fatalf("change threshold=%s", config.changeThreshold.RatString())
	}
	for label, values := range map[string]map[string]struct{}{
		"managed groups": config.managedGroups, "excluded groups": config.excludedGroups,
		"account types": config.accountTypes, "platforms": config.platforms,
		"paused accounts": config.pausedAccounts, "excluded accounts": config.excludedAccounts,
		"manual fused accounts": config.manualFusedAccounts,
	} {
		if len(values) != 1 {
			t.Fatalf("%s did not reach execution: %#v", label, values)
		}
	}
	if _, found := config.instantCodes[401]; !found || len(config.instantCodes) != 2 {
		t.Fatalf("instant status codes=%#v", config.instantCodes)
	}
	if _, found := config.cleanupStatusCodes[401]; !found || len(config.cleanupStatusCodes) != 2 {
		t.Fatalf("cleanup status codes=%#v", config.cleanupStatusCodes)
	}
}
