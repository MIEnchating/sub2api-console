package accountworkbench

import (
	"context"
	"encoding/json"
	"strings"
)

func equalEmail(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
func (s *Service) authorizeItem(ctx context.Context, value *privateRun, index int, active *activeRun, proxy string) (map[string]any, error) {
	return s.authorizeProtocol(ctx, value, index, active, proxy)
}
func publicCheck(result map[string]any, credentials map[string]any) map[string]any {
	raw, err := json.Marshal(result)
	if err != nil {
		return map[string]any{"verdict": "INCONCLUSIVE"}
	}
	encoded := string(raw)
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		if secret := text(credentials[key]); secret != "" {
			quoted, _ := json.Marshal(secret)
			escaped := string(quoted[1 : len(quoted)-1])
			encoded = strings.ReplaceAll(encoded, escaped, "[已隐藏]")
		}
	}
	output := map[string]any{}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.UseNumber()
	if decoder.Decode(&output) != nil {
		return map[string]any{"verdict": "INCONCLUSIVE"}
	}
	return output
}
