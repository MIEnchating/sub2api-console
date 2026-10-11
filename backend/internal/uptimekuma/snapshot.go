package uptimekuma

import (
	"context"
	"sync"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer release()
	c, err := s.store.UptimeKuma(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{Config: summary(c), Monitors: []Monitor{}}
	if c.BaseURL == "" {
		return result, nil
	}
	if c.Token == "" {
		result.Monitors, err = s.metrics(ctx, c)
		return result, err
	}

	// Both reads use the same configuration while the service gate is held.
	// A management failure cancels and joins the optional metrics read before
	// releasing that gate; a metrics failure still permits management fallback.
	ctx, cancel := context.WithCancel(ctx)
	var metrics []Monitor
	var metricsErr error
	var reads sync.WaitGroup
	reads.Go(func() { metrics, metricsErr = s.metrics(ctx, c) })
	defer func() {
		cancel()
		reads.Wait()
	}()
	result.Monitors, err = s.managementMonitors(ctx, c)
	if err != nil {
		return result, err
	}
	reads.Wait()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if metricsErr != nil {
		result.Warning = "指标读取失败，当前仅显示管理接口数据；请检查 API 密钥或重新保存接入配置"
		return result, nil
	}
	byID := map[int64]Monitor{}
	for _, m := range metrics {
		if m.ID > 0 {
			byID[m.ID] = m
		}
	}
	for i := range result.Monitors {
		m := &result.Monitors[i]
		if metric, ok := byID[m.ID]; ok {
			m.Status = metric.Status
			m.ResponseTime = metric.ResponseTime
			m.CertificateDays = metric.CertificateDays
			m.Uptime = metric.Uptime
		}
	}
	return result, nil
}

func (s *Service) managementMonitors(ctx context.Context, c configstore.UptimeKumaConfig) ([]Monitor, error) {
	conn, err := s.connect(ctx, c)
	if err != nil {
		return nil, err
	}
	defer conn.conn.CloseNow()
	if _, err = conn.call(ctx, "getMonitorList"); err != nil {
		return nil, err
	}
	if conn.monitors == nil {
		return nil, protocolError()
	}
	monitors, err := decodeMonitors(conn.monitors, conn.statuses)
	if err != nil {
		return nil, err
	}
	links, err := s.store.KumaMonitorTemplates(ctx, c.BaseURL)
	if err != nil {
		return monitors, err
	}
	for i := range monitors {
		link := links[monitors[i].ID]
		monitors[i].TemplateID = link.TemplateID
		monitors[i].TemplateRevision = link.Revision
		monitors[i].TemplateName = link.Name
		monitors[i].TemplateModel = link.Model
		monitors[i].TemplateBodyEncoding = link.BodyEncoding
	}
	return monitors, nil
}
