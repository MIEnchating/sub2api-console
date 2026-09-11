package officialpricing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Documentation requests have no credentials and may follow canonical redirects
// on the same HTTPS origin. Management clients retain their no-redirect policy.
func fetchURL(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	origin, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	public := *client
	public.Jar = nil
	public.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 || req.URL.Scheme != "https" || req.URL.Host != origin.Host || req.URL.User != nil {
			return errors.New("官方页面跳转超出允许范围")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Sub2API-Console/0.1")
	resp, err := public.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("官方价格 HTTP %d", resp.StatusCode)
	}
	const limit = 4 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if len(raw) > limit {
		return nil, errors.New("官方价格页面过大")
	}
	return raw, err
}
