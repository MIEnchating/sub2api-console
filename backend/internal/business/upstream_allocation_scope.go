package business

// UpstreamAllocationScope is shared by scheduling and the per-target editor.
// An explicit account setting wins over the upstream setting, then base scope.
type UpstreamAllocationScope struct {
	Enabled           bool
	Mode              string
	AccountIDs        map[string]bool
	UpstreamIDs       map[string]bool
	AccountOverrides  map[string]bool
	UpstreamOverrides map[string]bool
}

func ParseUpstreamAllocationScope(raw map[string]any) (UpstreamAllocationScope, error) {
	values, err := validateAdvancedSection("upstream_concurrency", raw)
	if err != nil {
		return UpstreamAllocationScope{}, err
	}
	result := UpstreamAllocationScope{Mode: "all"}
	result.Enabled, _ = values["enabled"].(bool)
	if mode, ok := values["account_mode"].(string); ok {
		result.Mode = mode
	}
	ids := func(key string) map[string]bool {
		out := map[string]bool{}
		list, _ := values[key].([]any)
		for _, id := range list {
			out[id.(string)] = true
		}
		return out
	}
	overrides := func(key string) map[string]bool {
		out := map[string]bool{}
		values, _ := values[key].(map[string]any)
		for id, value := range values {
			out[id] = value.(bool)
		}
		return out
	}
	result.AccountIDs, result.UpstreamIDs = ids("account_ids"), ids("upstream_ids")
	result.AccountOverrides, result.UpstreamOverrides = overrides("account_overrides"), overrides("upstream_overrides")
	return result, nil
}

func (s UpstreamAllocationScope) Selected(accountID, upstreamID string) (bool, string) {
	if value, ok := s.AccountOverrides[accountID]; accountID != "" && ok {
		return value, "account"
	}
	if value, ok := s.UpstreamOverrides[upstreamID]; upstreamID != "" && ok {
		return value, "upstream"
	}
	switch s.Mode {
	case "selected":
		return s.AccountIDs[accountID], "policy"
	case "upstreams":
		return s.UpstreamIDs[upstreamID], "policy"
	default:
		return true, "policy"
	}
}
