package uptimekuma

import (
	"context"
	"encoding/json"
	"strings"
)

// Retaining provenance never reloads the source template: it may have changed or been deleted.
func (s *Service) retainMonitorTemplate(ctx context.Context, baseURL string, id int64, in *MonitorInput, raw map[string]json.RawMessage) error {
	links, err := s.store.KumaMonitorTemplates(ctx, baseURL)
	if err != nil {
		return err
	}
	link, ok := links[id]
	if !ok || link.TemplateID != in.TemplateID || link.Revision != in.TemplateRevision {
		return failure("kuma_template_conflict", "监控的模板关联已更改，请刷新监控后重试", 409)
	}
	model := strings.TrimSpace(in.TemplateModel)
	if model != "" && model != link.Model {
		body, err := templateBodyWithModel(rawString(raw, "body"), rawString(raw, "httpBodyEncoding"), in.Type, model)
		if err != nil {
			return err
		}
		if in.Options == nil {
			in.Options = monitorOptions(raw)
		}
		in.Options.Body = body
		in.Options.ClearBody = false
		link.Model = model
	}
	in.appliedTemplate = &link
	return nil
}
