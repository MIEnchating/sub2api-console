package browserlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

// Remote talks only over the private shared Unix socket, never a browser URL.
type Remote struct{ client *http.Client }
type remoteBrowser struct {
	remote *Remote
	id     string
}
type wireResponse struct {
	ID     string                  `json:"id,omitempty"`
	Image  []byte                  `json:"image,omitempty"`
	Record *configstore.AuthRecord `json:"record,omitempty"`
	Error  string                  `json:"error,omitempty"`
}

func NewRemote(socket string) *Remote {
	return &Remote{client: &http.Client{Timeout: 45 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (r *Remote) call(ctx context.Context, method, path string, payload any) (wireResponse, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return wireResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://browser-worker"+path, bytes.NewReader(raw))
	if err != nil {
		return wireResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return wireResponse{}, ctx.Err()
		}
		return wireResponse{}, errors.New("验证浏览器服务连接失败，请确认 browser 容器已启动")
	}
	defer response.Body.Close()
	var value wireResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 12<<20)).Decode(&value); err != nil {
		return value, errors.New("验证浏览器服务响应无效")
	}
	if response.StatusCode != http.StatusOK {
		if value.Error != "" {
			return value, errors.New(value.Error)
		}
		return value, errors.New("验证浏览器操作失败")
	}
	return value, nil
}
func (r *Remote) Open(ctx context.Context, record configstore.AuthRecord) (Browser, error) {
	// Do not send existing credentials to the browser worker.
	candidate := configstore.AuthRecord{Host: record.Host, BaseURL: record.BaseURL, UpstreamType: record.UpstreamType}
	value, err := r.call(ctx, http.MethodPost, "/sessions", candidate)
	if err != nil {
		return nil, err
	}
	if len(value.ID) != 48 || strings.ContainsAny(value.ID, "/\\") {
		return nil, errors.New("验证浏览器会话响应无效")
	}
	return &remoteBrowser{remote: r, id: value.ID}, nil
}
func (b *remoteBrowser) Screenshot(ctx context.Context) ([]byte, error) {
	v, err := b.remote.call(ctx, http.MethodGet, "/sessions/"+b.id, nil)
	return v.Image, err
}
func (b *remoteBrowser) Input(ctx context.Context, v Input) error {
	_, err := b.remote.call(ctx, http.MethodPost, "/sessions/"+b.id+"/input", v)
	return err
}
func (b *remoteBrowser) Credentials(ctx context.Context) (configstore.AuthRecord, error) {
	v, err := b.remote.call(ctx, http.MethodPost, "/sessions/"+b.id+"/credentials", nil)
	if err != nil {
		return configstore.AuthRecord{}, err
	}
	if v.Record == nil {
		return configstore.AuthRecord{}, errors.New("浏览器尚未取得登录凭据")
	}
	return *v.Record, nil
}
func (b *remoteBrowser) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = b.remote.call(ctx, http.MethodDelete, "/sessions/"+b.id, nil)
}
