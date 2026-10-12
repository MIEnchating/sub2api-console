package adminclient

import (
	"context"
	"errors"
)

func (c *Client) Proxies(ctx context.Context) ([]map[string]any, error) {
	return c.fetchPaged(ctx, "/admin/proxies", "代理目录")
}

func (c *Client) Proxy(ctx context.Context, id string) (map[string]any, error) {
	if !stableID(id) {
		return nil, errors.New("代理 ID 必须是稳定数字 ID")
	}
	return c.resourceDetail(ctx, "/admin/proxies/"+id, id, "代理")
}
