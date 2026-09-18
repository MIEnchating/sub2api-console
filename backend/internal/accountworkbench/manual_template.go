package accountworkbench

import "encoding/json"

// Manual templates carry configuration only; no upstream identity is invented.
func manualTemplate(config TemplateConfig) Template {
	if config.GroupIDs == nil {
		config.GroupIDs = []string{}
	}
	if config.CredentialExtras == nil {
		config.CredentialExtras = map[string]json.RawMessage{}
	}
	if config.Extra == nil {
		config.Extra = map[string]json.RawMessage{}
	}
	summary := Account{Groups: []Group{}, ModelMapping: map[string]string{}, Concurrency: text(config.Concurrency), RateMultiplier: config.RateMultiplier}
	if config.LoadFactor != nil {
		summary.LoadFactor = *config.LoadFactor
	}
	if config.ProxyID != nil {
		summary.ProxyName = "代理 #" + *config.ProxyID
	}
	for _, id := range config.GroupIDs {
		summary.Groups = append(summary.Groups, Group{ID: id, Name: "分组 #" + id})
	}
	_ = json.Unmarshal(config.CredentialExtras["plan_type"], &summary.Plan)
	_ = json.Unmarshal(config.CredentialExtras["model_mapping"], &summary.ModelMapping)
	_ = json.Unmarshal(config.Extra["codex_fingerprint_mode"], &summary.Fingerprint)
	return Template{Config: config, Summary: summary}
}
