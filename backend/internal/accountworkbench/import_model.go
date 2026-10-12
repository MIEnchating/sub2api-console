package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const importModelKey = "settings:import-model"
const defaultImportModel = "gpt-6-astra"

type ImportModelSettings struct {
	Model    string `json:"model"`
	Revision int64  `json:"revision"`
}

func validImportModel(model string) bool {
	return model != "" && len(model) <= 200 && utf8.ValidString(model) && strings.IndexFunc(model, unicode.IsControl) < 0
}

func (s *Service) ImportModel(ctx context.Context) (ImportModelSettings, error) {
	result := ImportModelSettings{Model: defaultImportModel}
	raw, revision, err := s.private.WorkbenchDocument(ctx, importModelKey)
	if err != nil {
		return result, err
	}
	if len(raw) > 0 {
		if json.Unmarshal(raw, &result) != nil || !validImportModel(result.Model) {
			return ImportModelSettings{}, errors.New("检测模型设置无法读取，请检查配置")
		}
	}
	result.Revision = revision
	return result, nil
}

func (s *Service) SaveImportModel(ctx context.Context, input ImportModelSettings) (ImportModelSettings, error) {
	input.Model = strings.TrimSpace(input.Model)
	if !validImportModel(input.Model) || input.Revision < 0 {
		return ImportModelSettings{}, errors.New("请填写有效的检测模型（最多 200 字节，不含控制字符）")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return ImportModelSettings{}, err
	}
	input.Revision, err = s.private.SaveWorkbenchDocument(ctx, importModelKey, input.Revision, raw)
	return input, err
}
