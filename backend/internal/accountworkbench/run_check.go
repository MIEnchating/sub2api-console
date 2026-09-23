package accountworkbench

import (
	"context"
	"errors"
	"strings"
)

func passedImportCheck(result map[string]any) bool {
	if strings.TrimSpace(text(result["error"])) != "" {
		return false
	}
	switch text(result["verdict"]) {
	case "SOL_CONSISTENT", "MATCH", "GROUP_MATCH":
		return true
	default:
		return false
	}
}

func (s *Service) checkImportItem(ctx context.Context, value *privateRun, index int) error {
	row := &value.Public.Items[index]
	stored := &value.Items[index]
	row.Status = "checking"
	row.Message = "账号已导入，正在执行智商检测"
	row.Check = nil
	if err := s.persistRun(value); err != nil {
		return errors.New("检测阶段保存失败")
	}
	result, err := s.checker.CheckOAuthWithProxy(ctx, row.AccountID, stored.Item.Name, stored.Credentials, value.Settings.Model, 30, stored.ProxyURL)
	if err != nil {
		result = map[string]any{"verdict": "ERROR", "error": err.Error(), "claimed_model": value.Settings.Model}
	}
	row.Check = publicCheck(result, stored.Credentials)
	if row.Check == nil {
		row.Check = map[string]any{}
	}
	if !passedImportCheck(row.Check) {
		reason := strings.TrimSpace(text(row.Check["error"]))
		if reason == "" {
			switch text(row.Check["verdict"]) {
			case "INCONCLUSIVE":
				reason = "检测证据不足"
			case "LUNA_LIKE", "LUNA_CONSISTENT":
				reason = "检测结果更接近 Luna，未通过智商检测"
			case "TERRA_LIKE", "TERRA_CONSISTENT":
				reason = "检测结果更接近 Terra，未通过智商检测"
			case "MISMATCH":
				reason = "智商检测结果不匹配"
			default:
				reason = "检测未返回有效结果，请查看检测详情"
				row.Check["verdict"] = "ERROR"
				row.Check["error"] = reason
			}
		} else {
			row.Check["verdict"] = "ERROR"
		}
		row.Status = "review"
		row.Message = reason + "；账号已导入并保留模板分组，未开启调度，可手动启用"
	}
	return nil
}
