package uptimekuma

import (
	"context"
	"net/url"
)

func (s *Service) PushURL(ctx context.Context, id int64, revision string) (string, error) {
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	c, conn, err := s.resourceConnection(ctx)
	if err != nil {
		return "", err
	}
	defer conn.conn.CloseNow()
	r, err := conn.call(ctx, "getMonitor", id)
	if err != nil {
		return "", err
	}
	m, raw, err := decodeMonitor(r.Monitor)
	if err != nil {
		return "", err
	}
	if m.ID != id {
		return "", protocolError()
	}
	if revision == "" || revision != m.Revision {
		return "", failure("kuma_monitor_conflict", "监控项已更改，请刷新后重试", 409)
	}
	if m.Type != "push" {
		return "", failure("kuma_unsupported_type", "仅 Push 监控支持上报地址", 422)
	}
	token := rawString(raw, "pushToken")
	if token == "" {
		return "", protocolError()
	}
	return c.BaseURL + "/api/push/" + url.PathEscape(token) + "?status=up&msg=OK&ping=", nil
}
