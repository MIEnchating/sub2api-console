package uptimekuma

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"sort"
	"strconv"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type TemplateDistributionInput struct {
	MonitorIDs []int64 `json:"monitor_ids"`
	Revision   int64   `json:"revision"`
	Scope      string  `json:"scope"`
}

type TemplateDistributionItem struct {
	MonitorID        int64  `json:"monitor_id"`
	Name             string `json:"name"`
	TemplateRevision int64  `json:"template_revision"`
	Reason           string `json:"reason"`
	revision         string
	link             configstore.KumaMonitorTemplate
}

type TemplateDistributionPreview struct {
	TemplateID     string                     `json:"template_id"`
	Revision       int64                      `json:"revision"`
	ConfigRevision int64                      `json:"config_revision"`
	Scope          string                     `json:"scope"`
	Items          []TemplateDistributionItem `json:"items"`
}

func (s *Service) PreviewTemplateDistribution(ctx context.Context, id string, revision int64) (TemplateDistributionPreview, error) {
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return TemplateDistributionPreview{}, err
	}
	defer release()
	item, err := s.distributionTemplate(ctx, id, revision)
	if err != nil {
		return TemplateDistributionPreview{}, err
	}
	c, err := s.store.UptimeKuma(ctx)
	if err != nil {
		return TemplateDistributionPreview{}, err
	}
	links, err := s.store.KumaMonitorTemplates(ctx, c.BaseURL)
	if err != nil {
		return TemplateDistributionPreview{}, err
	}
	conn, err := s.connect(ctx, c)
	if err != nil {
		return TemplateDistributionPreview{}, err
	}
	defer conn.conn.CloseNow()
	if _, err = conn.call(ctx, "getMonitorList"); err != nil {
		return TemplateDistributionPreview{}, err
	}
	if conn.monitors == nil {
		return TemplateDistributionPreview{}, protocolError()
	}
	preview := TemplateDistributionPreview{TemplateID: id, Revision: revision, ConfigRevision: c.Revision, Items: []TemplateDistributionItem{}}
	for monitorID, link := range links {
		if link.TemplateID != id {
			continue
		}
		entry := TemplateDistributionItem{MonitorID: monitorID, Name: "监控 #" + strconv.FormatInt(monitorID, 10), TemplateRevision: link.Revision, link: link}
		data, exists := conn.monitors[strconv.FormatInt(monitorID, 10)]
		if !exists {
			entry.Reason = "监控已不存在，跳过"
		} else {
			monitor, raw, err := decodeMonitor(data)
			if err != nil {
				return TemplateDistributionPreview{}, err
			}
			if monitor.ID != monitorID {
				return TemplateDistributionPreview{}, protocolError()
			}
			entry.Name = monitor.Name
			entry.revision = distributionMonitorRevision(raw)
			if _, err := distributionMonitorInput(item, link, monitor, raw); err != nil {
				entry.Reason = PublicError(err).Message
			}
		}
		preview.Items = append(preview.Items, entry)
	}
	sort.Slice(preview.Items, func(i, j int) bool { return preview.Items[i].MonitorID < preview.Items[j].MonitorID })
	// Include private provenance and monitor revisions in the opaque scope, never their contents.
	parts := []any{id, revision, c.Revision}
	for _, entry := range preview.Items {
		parts = append(parts, entry, entry.revision, entry.link)
	}
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	preview.Scope = hex.EncodeToString(sum[:])
	return preview, nil
}

func (s *Service) distributionTemplate(ctx context.Context, id string, revision int64) (configstore.KumaTemplate, error) {
	if !templateIDPattern.MatchString(id) || revision < 1 {
		return configstore.KumaTemplate{}, failure("kuma_invalid_template", "模板 ID 或版本无效", 422)
	}
	item, err := s.store.KumaTemplate(ctx, id)
	if err != nil {
		return item, templateError(err)
	}
	if item.Revision != revision {
		return item, templateError(configstore.ErrKumaTemplateConflict)
	}
	return item, nil
}

func distributionMonitorRevision(raw map[string]json.RawMessage) string {
	return monitorRevision(raw) + ":" + rawString(raw, "httpBodyEncoding")
}

func distributionMonitorInput(item configstore.KumaTemplate, link configstore.KumaMonitorTemplate, current Monitor, raw map[string]json.RawMessage) (MonitorInput, error) {
	in := MonitorInput{editing: true, Name: current.Name, Type: current.Type, Interval: current.Interval, Parent: current.Parent, URL: rawString(raw, "url"), Options: current.Options, TemplateID: item.ID, TemplateRevision: item.Revision, TemplateModel: link.Model}
	if item.Monitoring != nil && item.Monitoring.Type != current.Type {
		return in, failure("kuma_template_type_changed", "模板与监控类型不同，请单独编辑或新建监控", 409)
	}
	if err := applyResolvedTemplate(item, &in); err != nil {
		return in, err
	}
	// Destinations and credentials belong to each monitor, even when the template changes.
	in.URL = rawString(raw, "url")
	in.Options.Hostname = rawString(raw, "hostname")
	in.Options.Port = rawInt(raw, "port", 0)
	authRaw := maps.Clone(raw)
	if rawString(authRaw, "authMethod") == "" {
		authRaw["authMethod"] = json.RawMessage(`"none"`)
	}
	if err := prepareMonitorAuthEdit(authRaw, &in); err != nil {
		return in, err
	}
	return in, validateMonitor(in)
}

func (s *Service) distributeTemplateMonitor(ctx context.Context, preview TemplateDistributionPreview, expected TemplateDistributionItem) (bool, error) {
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	c, err := s.store.UptimeKuma(ctx)
	if err != nil {
		return false, err
	}
	if c.Revision != preview.ConfigRevision {
		return false, configstore.ErrKumaConfigConflict
	}
	item, err := s.distributionTemplate(ctx, preview.TemplateID, preview.Revision)
	if err != nil {
		return false, err
	}
	link, err := s.store.KumaMonitorTemplate(ctx, c.BaseURL, expected.MonitorID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, distributionConflict()
	}
	if err != nil {
		return false, err
	}
	if link != expected.link {
		return false, distributionConflict()
	}
	conn, err := s.connect(ctx, c)
	if err != nil {
		return false, err
	}
	defer conn.conn.CloseNow()
	r, err := conn.call(ctx, "getMonitor", expected.MonitorID)
	if err != nil {
		return false, err
	}
	current, raw, err := decodeMonitor(r.Monitor)
	if err != nil {
		return false, err
	}
	if current.ID != expected.MonitorID || distributionMonitorRevision(raw) != expected.revision {
		return false, distributionConflict()
	}
	in, err := distributionMonitorInput(item, expected.link, current, raw)
	if err != nil {
		return false, err
	}
	// Preserve the exact remote notification mapping and every unrelated field.
	preserved := map[string]json.RawMessage{}
	for _, key := range []string{"notificationIDList", "authMethod", "basic_auth_user", "basic_auth_pass", "bearer_token"} {
		preserved[key] = raw[key]
	}
	applyOptions(raw, current.Type, in.Options)
	for key, value := range preserved {
		if value == nil {
			delete(raw, key)
		} else {
			raw[key] = value
		}
	}
	raw["interval"], _ = json.Marshal(in.Interval)
	if _, err = conn.call(ctx, "editMonitor", raw); err != nil {
		return true, err
	}
	in.appliedTemplate.MonitorID = current.ID
	if err = s.store.SaveKumaMonitorTemplate(ctx, c.BaseURL, *in.appliedTemplate); err != nil {
		return true, failure("kuma_template_record_failed", "远端已更新，但模板关联保存失败；请刷新核对后再操作", 500)
	}
	return true, nil
}

func distributionConflict() error {
	return failure("kuma_distribution_conflict", "模板关联或监控参数已更改，请重新预览后下发", 409)
}
