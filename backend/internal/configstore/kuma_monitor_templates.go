package configstore

import "context"

// KumaMonitorTemplate stores provenance only. It contains no headers, bodies or credentials.
type KumaMonitorTemplate struct {
	MonitorID    int64
	TemplateID   string
	Revision     int64
	Name         string
	Model        string
	BodyEncoding string
}

func (s *Store) KumaMonitorTemplates(ctx context.Context, baseURL string) (map[int64]KumaMonitorTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT monitor_id,template_id,template_revision,template_name,template_model,body_encoding FROM uptime_kuma_monitor_templates WHERE base_url=?`, baseURL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[int64]KumaMonitorTemplate{}
	for rows.Next() {
		var item KumaMonitorTemplate
		if err = rows.Scan(&item.MonitorID, &item.TemplateID, &item.Revision, &item.Name, &item.Model, &item.BodyEncoding); err != nil {
			return nil, err
		}
		items[item.MonitorID] = item
	}
	return items, rows.Err()
}

func (s *Store) SaveKumaMonitorTemplate(ctx context.Context, baseURL string, item KumaMonitorTemplate) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO uptime_kuma_monitor_templates(base_url,monitor_id,template_id,template_revision,template_name,template_model,body_encoding) VALUES(?,?,?,?,?,?,?) ON CONFLICT(base_url,monitor_id) DO UPDATE SET template_id=excluded.template_id,template_revision=excluded.template_revision,template_name=excluded.template_name,template_model=excluded.template_model,body_encoding=excluded.body_encoding`, baseURL, item.MonitorID, item.TemplateID, item.Revision, item.Name, item.Model, item.BodyEncoding)
	return err
}

func (s *Store) DeleteKumaMonitorTemplate(ctx context.Context, baseURL string, monitorID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM uptime_kuma_monitor_templates WHERE base_url=? AND monitor_id=?`, baseURL, monitorID)
	return err
}
