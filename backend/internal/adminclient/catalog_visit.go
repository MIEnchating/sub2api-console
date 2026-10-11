package adminclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// VisitAccounts consumes validated pages without retaining the complete raw
// catalog. Callbacks must stage results locally: a later page can still fail,
// and callers must discard staged results unless this method returns nil.
// Returning an error stops consumption and further requests.
func (c *Client) VisitAccounts(ctx context.Context, visit func(map[string]any) error) error {
	if visit == nil {
		return errors.New("账号目录读取回调不能为空")
	}
	return c.visitPaged(ctx, "/admin/accounts", "账号目录", visit)
}

func (c *Client) visitPaged(ctx context.Context, path, label string, visit func(map[string]any) error) error {
	seen := map[string]struct{}{}
	count := 0
	var declaredTotal *int
	for page := 1; page <= 10000; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		payload, err := c.request(ctx, http.MethodGet, path, nil, map[string]string{"page": strconv.Itoa(page), "page_size": strconv.Itoa(catalogPageSize)})
		if err != nil {
			message := fmt.Sprintf("%s %s 第 %d 页读取失败（每页 %d 项，已读取 %d 项）", label, path, page, catalogPageSize, count)
			var timeoutError net.Error
			if ctx.Err() == nil && (errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeoutError) && timeoutError.Timeout())) {
				message += "：请求超时，请检查管理端负载和网络，必要时调整管理 API 请求超时设置"
			}
			return fmt.Errorf("%s：%w", message, err)
		}
		pageItems, metadata, err := items(payload, label)
		if err != nil {
			return err
		}
		for _, item := range pageItems {
			id := strings.TrimSpace(fmt.Sprint(item["id"]))
			if !stableID(id) {
				return &Error{label + "项目缺少稳定 ID"}
			}
			if _, ok := seen[id]; ok {
				return &Error{label + "返回重复 ID"}
			}
			seen[id] = struct{}{}
		}
		total, present, err := pageTotal(metadata, label)
		if err != nil {
			return err
		}
		if present {
			if declaredTotal != nil && *declaredTotal != total {
				return &Error{fmt.Sprintf("%s 分页总数在读取期间发生变化", label)}
			}
			declaredTotal = &total
		}
		count += len(pageItems)
		if declaredTotal != nil && count > *declaredTotal {
			return &Error{fmt.Sprintf("%s 分页返回项目数超过声明总数", label)}
		}
		if len(pageItems) == 0 && declaredTotal != nil && count != *declaredTotal {
			return &Error{fmt.Sprintf("%s 分页未完整：已读取 %d/%d 项", label, count, *declaredTotal)}
		}
		for _, item := range pageItems {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := visit(item); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(pageItems) == 0 || (declaredTotal != nil && count == *declaredTotal) {
			return nil
		}
	}
	return &Error{label + "分页超过安全上限"}
}
