package accountworkbench

import (
	"context"
	"errors"
	"strings"
)

func completedImportCheck(result map[string]any) bool {
	if strings.TrimSpace(text(result["error"])) != "" {
		return false
	}
	switch text(result["verdict"]) {
	case "SOL_CONSISTENT", "LUNA_LIKE", "TERRA_LIKE", "LUNA_CONSISTENT", "TERRA_CONSISTENT", "INCONCLUSIVE", "MATCH", "MISMATCH":
		return true
	default:
		return false
	}
}

func (s *Service) checkImportItem(ctx context.Context, value *privateRun, index int) error {
	row := &value.Public.Items[index]
	stored := &value.Items[index]
	row.Status = "checking"
	row.Message = "正在导入前执行检测"
	row.Check = nil
	if err := s.persistRun(value); err != nil {
		return errors.New("检测阶段保存失败")
	}
	// This item exists only in the private batch until its check completes. Do not
	// create a managed account just to obtain an ID for the detection report.
	result, err := s.checker.CheckOAuthWithProxy(ctx, row.ID, stored.Item.Name, stored.Credentials, value.Settings.Model, 30, stored.ProxyURL)
	if err != nil {
		result = map[string]any{"verdict": "ERROR", "error": err.Error(), "claimed_model": value.Settings.Model}
	}
	row.Check = publicCheck(result, stored.Credentials)
	if row.Check == nil {
		row.Check = map[string]any{}
	}
	if !completedImportCheck(row.Check) {
		reason := strings.TrimSpace(text(row.Check["error"]))
		if reason == "" {
			reason = "检测未返回有效结果，请核对检测配置后重试"
		}
		row.Check["verdict"] = "ERROR"
		row.Check["error"] = reason
		return errors.New("检测出错，已停止导入：" + reason)
	}
	return nil
}
