package modelcheck

import (
	"context"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type profileAccounts struct {
	fakeAccounts
	raw     []byte
	actions []string
	actors  []string
}

func (accounts *profileAccounts) LoadModelCheckConfiguration(context.Context) ([]byte, error) {
	return append([]byte(nil), accounts.raw...), nil
}

func (accounts *profileAccounts) SaveModelCheckConfiguration(_ context.Context, raw []byte, actor, action string) error {
	accounts.raw = append([]byte(nil), raw...)
	accounts.actors = append(accounts.actors, actor)
	accounts.actions = append(accounts.actions, action)
	return nil
}

func newConfigurationService(t *testing.T, accounts *profileAccounts) *Service {
	t.Helper()
	service, err := New(
		&recordingTasks{terminal: make(chan taskstore.Task, 1)},
		&fakeCredentials{},
		accounts,
		&fakeKeyRevealer{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestConfigurationDraftPublishAndReloadPreserveVersionHistory(t *testing.T) {
	accounts := &profileAccounts{}
	service := newConfigurationService(t, accounts)
	builtin := service.Configuration()
	payload := builtin.Active.Payload
	payload.SolProfile.CandidateModels = []string{"custom-sol", "custom-luna", "custom-terra"}

	draft, err := service.SaveDraft(context.Background(), SaveDraftRequest{
		ExpectedFingerprint: builtin.Active.Fingerprint,
		Note:                "自定义模型画像",
		Payload:             payload,
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if draft.Draft == nil || draft.Draft.Status != "draft" || draft.Draft.Fingerprint == builtin.Active.Fingerprint {
		t.Fatalf("draft=%#v", draft.Draft)
	}
	if capabilities := service.Capabilities(); capabilities.SolModels[0] != "gpt-5.6-sol" {
		t.Fatalf("draft changed active capabilities: %#v", capabilities)
	}

	published, err := service.PublishDraft(context.Background(), PublishRequest{
		ExpectedFingerprint: draft.Draft.Fingerprint,
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if published.Draft != nil || published.Active.Status != "published" || published.Active.Note != "自定义模型画像" ||
		len(published.History) != 1 || published.History[0].ID != "builtin" {
		t.Fatalf("published=%#v", published)
	}
	if capabilities := service.Capabilities(); capabilities.SolModels[0] != "custom-sol" {
		t.Fatalf("published capabilities=%#v", capabilities)
	}
	if len(accounts.actions) != 2 || accounts.actions[0] != "draft.saved" || accounts.actions[1] != "version.published" ||
		accounts.actors[0] != "operator" {
		t.Fatalf("actions=%#v actors=%#v", accounts.actions, accounts.actors)
	}

	reloaded := newConfigurationService(t, accounts)
	if view := reloaded.Configuration(); view.Active.ID != published.Active.ID || len(view.History) != 1 {
		t.Fatalf("reloaded=%#v", view)
	}
}

func TestConfigurationRejectsInvalidWeightsAndStaleWrites(t *testing.T) {
	accounts := &profileAccounts{}
	service := newConfigurationService(t, accounts)
	view := service.Configuration()
	payload := view.Active.Payload
	for name, profile := range payload.ClaudeProfiles {
		profile.Probes[0].Weights = map[string][]float64{"o0": {0}}
		payload.ClaudeProfiles[name] = profile
		break
	}
	_, err := service.SaveDraft(context.Background(), SaveDraftRequest{
		ExpectedFingerprint: view.Active.Fingerprint,
		Payload:             payload,
	}, "operator")
	if err == nil || !strings.Contains(err.Error(), "权重数量") {
		t.Fatalf("invalid weights error=%v", err)
	}
	if len(accounts.raw) != 0 {
		t.Fatal("invalid configuration was persisted")
	}

	view = service.Configuration()
	_, err = service.SaveDraft(context.Background(), SaveDraftRequest{
		ExpectedFingerprint: "stale",
		Payload:             view.Active.Payload,
	}, "operator")
	if err == nil || !strings.Contains(err.Error(), "已变化") {
		t.Fatalf("stale write error=%v", err)
	}
}

func TestPreparedRunKeepsPublishedProfileSnapshot(t *testing.T) {
	accounts := &profileAccounts{fakeAccounts: fakeAccounts{
		rows:    []business.AccountStatus{{ID: "41", Name: "主账号"}},
		details: map[string]*business.AccountDetail{"41": {}},
	}}
	service := newConfigurationService(t, accounts)
	prepared, err := service.prepare(context.Background(), Request{
		AccountIDs: []string{"41"}, Models: []string{"gpt-5.6-sol"},
	})
	if err != nil {
		t.Fatal(err)
	}
	originalID := prepared.profileVersion
	view := service.Configuration()
	payload := view.Active.Payload
	payload.SolProfile.CandidateModels = []string{"custom-sol", "custom-luna", "custom-terra"}
	draft, err := service.SaveDraft(context.Background(), SaveDraftRequest{
		ExpectedFingerprint: view.Active.Fingerprint, Payload: payload,
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishDraft(context.Background(), PublishRequest{ExpectedFingerprint: draft.Draft.Fingerprint}, "operator"); err != nil {
		t.Fatal(err)
	}
	if prepared.profileVersion != originalID || checkerForModel("gpt-5.6-sol", prepared.claudeProfiles, prepared.solProfile) != "sol" ||
		service.checkerForModel("gpt-5.6-sol") != "" {
		t.Fatalf("prepared profile was not isolated: version=%q checker=%q", prepared.profileVersion, service.checkerForModel("gpt-5.6-sol"))
	}
}
