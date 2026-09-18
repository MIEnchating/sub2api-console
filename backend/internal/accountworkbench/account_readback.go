package accountworkbench

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func appliedValue(actual, expected any) bool {
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
