package probe

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestFirstProbeTargetPerAccountRunsOnlyOneConfiguredModelPerAccount(t *testing.T) {
	modelA, modelB := "model-a", "model-b"
	result := firstProbeTargetPerAccount([]Target{
		{AccountID: "41", GroupName: "group-a", Model: &modelA},
		{AccountID: "41", GroupName: "group-b", Model: &modelB},
		{AccountID: "42", GroupName: "group-a", Model: &modelB},
		{AccountID: "43", GroupName: "group-a"},
		{AccountID: "43", GroupName: "group-b", Model: &modelA},
	})
	if len(result) != 3 || result[0].AccountID != "41" || *result[0].Model != modelA || result[1].AccountID != "42" {
		t.Fatalf("collapsed probe targets=%#v", result)
	}
	if result[2].AccountID != "43" || result[2].GroupName != "group-b" || result[2].Model == nil || *result[2].Model != modelA {
		t.Fatalf("executable target was not preferred: %#v", result[2])
	}
}

func TestExplicitProbeModelRunsOnlyAccountsThatHaveThatEnabledModel(t *testing.T) {
	targets, err := buildTargets([]business.ProbeCandidate{
		{AccountID: "41", GroupName: "default", KnownModels: []string{"model-a"}, Metadata: map[string]any{}},
		{AccountID: "42", GroupName: "default", KnownModels: []string{"model-a", "Model-B"}, Metadata: map[string]any{}},
	}, map[string]any{}, targetOptions{probeModel: "model-b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].SkipReason == nil || targets[0].Model == nil || *targets[0].Model != "model-b" {
		t.Fatalf("account without selected model was not preserved as skipped: %#v", targets)
	}
	if targets[1].SkipReason != nil || targets[1].Model == nil || *targets[1].Model != "Model-B" {
		t.Fatalf("account with selected model did not use its canonical model: %#v", targets[1])
	}
}

func TestBuildTargetsExcludesManuallyFusedAccounts(t *testing.T) {
	targets, err := buildTargets([]business.ProbeCandidate{
		{AccountID: "41", GroupName: "default", KnownModels: []string{"model-a"}, Metadata: map[string]any{}},
		{AccountID: "42", GroupName: "default", KnownModels: []string{"model-a"}, Metadata: map[string]any{}},
	}, map[string]any{
		"scope": map[string]any{"manual_fused_account_ids": []any{"41"}},
	}, targetOptions{probeModel: "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].AccountID != "42" {
		t.Fatalf("manually fused account entered probe targets: %#v", targets)
	}
}

func TestForcedPlatformProbeUsesInputModelWithoutCatalogPrecheck(t *testing.T) {
	targets, err := buildTargets([]business.ProbeCandidate{
		{AccountID: "41", GroupName: "default", KnownModels: []string{"model-a"}, Metadata: map[string]any{}},
		{AccountID: "42", GroupName: "default", KnownModels: nil, Metadata: map[string]any{}},
	}, map[string]any{}, targetOptions{probeModel: "unlisted-model", forceProbeModel: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets=%#v", targets)
	}
	for _, target := range targets {
		if target.SkipReason != nil || target.Model == nil || *target.Model != "unlisted-model" {
			t.Fatalf("input model was not forced for account %s: %#v", target.AccountID, target)
		}
	}
}

func TestAccountProbeModelsSelectOneDifferentModelPerAccount(t *testing.T) {
	targets, err := buildTargets([]business.ProbeCandidate{
		{AccountID: "41", GroupName: "default", KnownModels: []string{"model-a"}, Metadata: map[string]any{}},
		{AccountID: "42", GroupName: "default", KnownModels: []string{"model-a", "Model-B"}, Metadata: map[string]any{}},
	}, map[string]any{}, targetOptions{probeModels: map[string]string{"41": "model-a", "42": "model-b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].Model == nil || *targets[0].Model != "model-a" {
		t.Fatalf("account 41 target=%#v", targets)
	}
	if targets[1].Model == nil || *targets[1].Model != "Model-B" || targets[1].SkipReason != nil {
		t.Fatalf("account 42 target=%#v", targets[1])
	}
}

func TestSkippedExplicitProbeResultKeepsSelectedModel(t *testing.T) {
	model := "model-b"
	reason := "所选实际验证模型不在该账号的已启用模型中"
	result := probeTarget(nil, nil, Target{
		AccountID: "41", GroupName: "default", Model: &model, SkipReason: &reason,
	}, Config{}, RetryConfig{})
	if result.Result != "跳过" || result.RequestModel != model || result.FailureReason == nil || *result.FailureReason != reason {
		t.Fatalf("skipped result lost selected probe model: %#v", result)
	}
}

func TestProbeResultKeepsAccountName(t *testing.T) {
	model := "model-a"
	reason := "not available"
	result := probeTarget(nil, nil, Target{
		AccountID: "41", AccountName: "主账号", GroupName: "default", Model: &model, SkipReason: &reason,
	}, Config{}, RetryConfig{})
	if result.AccountName != "主账号" {
		t.Fatalf("account name missing from result: %#v", result)
	}
}
