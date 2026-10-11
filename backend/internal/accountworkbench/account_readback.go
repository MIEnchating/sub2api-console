package accountworkbench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/MIEnchating/sub2api-console/backend/internal/decimalutil"
)

// verifyApplied checks every requested field before scheduling can be enabled.
// Maps may contain server-owned additions; requested numbers retain exact precision.
func verifyApplied(account, requested map[string]any) error {
	for key, expected := range requested {
		if key == "skip_default_group_bind" {
			continue
		}
		if key == "group_ids" {
			var ids []any
			raw, _ := json.Marshal(expected)
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			if decoder.Decode(&ids) != nil {
				return fmt.Errorf("账号分组配置无法核对")
			}
			wanted := make([]string, 0, len(ids))
			for _, id := range ids {
				wanted = append(wanted, text(id))
			}
			actual := []string{}
			for _, group := range publicAccount(account).Groups {
				actual = append(actual, group.ID)
			}
			slices.Sort(wanted)
			slices.Sort(actual)
			if !slices.Equal(wanted, actual) {
				return fmt.Errorf("账号分组配置未生效，已停止启用")
			}
			continue
		}
		if key == "credentials" {
			if err := verifyModelMappings(account, object(expected)); err != nil {
				return err
			}
		}
		if key == "expires_at" && expected != nil {
			wanted, wantedErr := accountExpirySeconds(text(expected))
			actual, actualErr := accountExpirySeconds(text(account[key]))
			if wantedErr == nil && ((wanted == 0 && account[key] == nil) || (actualErr == nil && wanted == actual)) {
				continue
			}
			return fmt.Errorf("账号到期时间未生效，已停止启用")
		}
		if !appliedValue(account[key], expected) {
			return fmt.Errorf("账号配置 %s 未生效，已停止启用", key)
		}
	}
	return nil
}

// verifyImportedApplied validates the parts of an import that the management
// API can read back. OAuth secrets are intentionally redacted by the API, so
// comparing the full credentials object would turn a successful import into a
// false "credentials not applied" failure. Stable identity is checked by the
// caller and model mappings remain safe, non-secret configuration to verify.
func verifyImportedApplied(account, requested map[string]any) error {
	if credentials, ok := requested["credentials"].(map[string]any); ok {
		if err := verifyImportedCredentialState(account, credentials); err != nil {
			return err
		}
		if err := verifyModelMappings(account, credentials); err != nil {
			return err
		}
	}
	withoutCredentials := maps.Clone(requested)
	delete(withoutCredentials, "credentials")
	return verifyApplied(account, withoutCredentials)
}

func verifyImportedAppliedWhileIsolated(account, requested map[string]any) error {
	withoutStatus := maps.Clone(requested)
	delete(withoutStatus, "status")
	return verifyImportedApplied(account, withoutStatus)
}

func verifyImportedCredentialState(account, expected map[string]any) error {
	status := object(account["credentials_status"])
	for _, field := range []struct {
		credential string
		statusKey  string
	}{
		{credential: "access_token", statusKey: "has_access_token"},
		{credential: "refresh_token", statusKey: "has_refresh_token"},
	} {
		if text(expected[field.credential]) == "" {
			continue
		}
		if configured, ok := status[field.statusKey].(bool); ok && !configured {
			return fmt.Errorf("账号凭据 %s 未生效，已停止启用", field.credential)
		}
	}
	return nil
}

func appliedValue(actual, expected any) bool {
	// Sub2API serializes an explicitly empty optional text field as JSON null.
	// Treat the two representations as equivalent so a successful write is not
	// incorrectly classified as uncertain during the isolation readback.
	if wanted, ok := expected.(string); ok && wanted == "" && actual == nil {
		return true
	}
	if wanted, ok := expected.(map[string]any); ok {
		got, ok := actual.(map[string]any)
		if !ok {
			return len(wanted) == 0 && actual == nil
		}
		for key, value := range wanted {
			if !appliedValue(got[key], value) {
				return false
			}
		}
		return true
	}
	if wanted, ok := expected.(json.Number); ok {
		left, leftOK := decimalutil.Parse(text(actual))
		right, rightOK := decimalutil.Parse(wanted.String())
		return leftOK && rightOK && left.Cmp(right) == 0
	}
	return reflect.DeepEqual(actual, expected)
}

// Model selections are exact sets; upstream-added wildcard entries must not widen them.
func verifyModelMappings(account map[string]any, expected map[string]any) error {
	actual := object(account["credentials"])
	for _, key := range []string{"model_mapping", "compact_model_mapping"} {
		wanted, exists := expected[key]
		if !exists {
			continue
		}
		got := actual[key]
		if len(object(wanted)) == 0 && got == nil {
			continue
		}
		if !reflect.DeepEqual(got, wanted) {
			return fmt.Errorf("账号模型映射 %s 与所选配置不一致，已停止启用", key)
		}
	}
	return nil
}
