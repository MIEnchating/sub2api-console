package uptimekuma

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type Service struct {
	store  *configstore.Store
	client *http.Client
	gate   chan struct{}
}

func New(store *configstore.Store, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Service{store: store, client: &copyClient, gate: make(chan struct{}, 1)}
}
func (s *Service) enter(ctx context.Context) (context.Context, func(), error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	select {
	case s.gate <- struct{}{}:
		return ctx, func() { <-s.gate; cancel() }, nil
	case <-ctx.Done():
		cancel()
		return nil, nil, failure("kuma_busy", "Uptime Kuma 操作正在处理中，请稍后重试", 409)
	}
}
func (s *Service) Config(ctx context.Context) (Config, error) {
	c, err := s.store.UptimeKuma(ctx)
	return summary(c), err
}
func (s *Service) Save(ctx context.Context, in ConfigInput) (Config, error) {
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return Config{}, err
	}
	defer release()
	c, err := s.store.UptimeKuma(ctx)
	if err != nil {
		return Config{}, err
	}
	if in.Revision != c.Revision {
		return Config{}, configstore.ErrKumaConfigConflict
	}
	base, err := configstore.ValidateBaseURL(strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(in.BaseURL), "/"), "/dashboard"))
	if err != nil || len(base) > 2048 {
		return Config{}, failure("kuma_invalid_url", "请填写完整的 http 或 https 服务地址，不含查询参数或登录凭据", 422)
	}
	in.APIKey = strings.TrimSpace(in.APIKey)
	in.Username = strings.TrimSpace(in.Username)
	if len(in.APIKey) > 4096 || len(in.Username) > 255 || len(in.Password) > 4096 || len(in.OTP) > 6 {
		return Config{}, failure("kuma_invalid_credentials", "凭据长度无效，请检查输入", 422)
	}
	changed := c.BaseURL != base
	if changed && in.APIKey == "" {
		return Config{}, failure("kuma_key_required", "首次接入或更换地址时必须重新输入 API 密钥", 422)
	}
	if changed {
		c.APIKey = ""
		c.Username = ""
		c.Password = ""
		c.Token = ""
	}
	c.BaseURL = base
	if in.APIKey != "" {
		c.APIKey = in.APIKey
	}
	if c.APIKey == "" {
		return Config{}, failure("kuma_key_required", "请输入 API 密钥", 422)
	}
	if in.DisableManagement {
		c.Username = ""
		c.Password = ""
		c.Token = ""
	} else {
		if c.Username != in.Username {
			c.Password = ""
			c.Token = ""
		}
		c.Username = in.Username
		if in.Password != "" {
			c.Password = in.Password
			c.Token = ""
		}
		if (c.Username == "") != (c.Password == "") {
			return Config{}, failure("kuma_credentials_required", "请同时填写管理账号和密码，或清空账号以仅查看指标", 422)
		}
		if c.Username == "" {
			c.Token = ""
		}
	}
	if _, err = s.metrics(ctx, c); err != nil {
		return Config{}, err
	}
	if c.Username != "" {
		conn, err := dial(ctx, c.BaseURL, s.client)
		if err != nil {
			return Config{}, err
		}
		defer conn.conn.CloseNow()
		if c.Token != "" && in.OTP == "" {
			_, err = conn.call(ctx, "loginByToken", c.Token)
		} else {
			var r reply
			r, err = conn.call(ctx, "login", struct {
				Username string `json:"username"`
				Password string `json:"password"`
				Token    string `json:"token"`
			}{c.Username, c.Password, in.OTP})
			c.Token = r.Token
		}
		if err != nil {
			return Config{}, err
		}
		if c.Token == "" {
			return Config{}, protocolError()
		}
	}
	if err = s.store.SaveUptimeKuma(ctx, c); err != nil {
		return Config{}, err
	}
	c.Revision++
	return summary(c), nil
}
func (s *Service) Disconnect(ctx context.Context, revision int64) error {
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.store.SaveUptimeKuma(ctx, configstore.UptimeKumaConfig{Revision: revision})
}
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
	metrics, metricsErr := s.metrics(ctx, c)
	if c.Token == "" {
		result.Monitors = metrics
		return result, metricsErr
	}
	conn, err := s.connect(ctx, c)
	if err != nil {
		return result, err
	}
	defer conn.conn.CloseNow()
	if _, err = conn.call(ctx, "getMonitorList"); err != nil {
		return result, err
	}
	if conn.monitors == nil {
		return result, protocolError()
	}
	result.Monitors, err = decodeMonitors(conn.monitors, conn.statuses)
	if err != nil {
		return result, err
	}
	links, err := s.store.KumaMonitorTemplates(ctx, c.BaseURL)
	if err != nil {
		return result, err
	}
	for i := range result.Monitors {
		link := links[result.Monitors[i].ID]
		result.Monitors[i].TemplateID = link.TemplateID
		result.Monitors[i].TemplateRevision = link.Revision
		result.Monitors[i].TemplateName = link.Name
		result.Monitors[i].TemplateModel = link.Model
		result.Monitors[i].TemplateBodyEncoding = link.BodyEncoding
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
func (s *Service) connect(ctx context.Context, c configstore.UptimeKumaConfig) (*socket, error) {
	if c.Token == "" {
		return nil, failure("kuma_management_required", "请先在接入配置中填写账号密码并验证，API 密钥仅支持查看指标", 409)
	}
	conn, err := dial(ctx, c.BaseURL, s.client)
	if err != nil {
		return nil, err
	}
	if _, err = conn.call(ctx, "loginByToken", c.Token); err != nil {
		conn.conn.CloseNow()
		return nil, err
	}
	return conn, nil
}
func PublicError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	if errors.Is(err, configstore.ErrKumaConfigConflict) {
		return &Error{"kuma_config_conflict", err.Error(), 409}
	}
	return &Error{"kuma_internal_error", "接入配置处理失败，请稍后重试", 500}
}
