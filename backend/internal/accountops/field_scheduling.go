package accountops

import (
	"context"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func syncAccountScheduling(ctx context.Context, client *adminclient.Client, accountID string, after map[string]any, desired *bool) (map[string]any, error) {
	if desired == nil {
		return after, nil
	}
	current, err := accountSchedulable(after)
	if err != nil {
		return nil, err
	}
	if current == *desired {
		return after, nil
	}
	if err := writeAccountSchedulable(ctx, client, accountID, *desired); err != nil {
		return nil, err
	}
	confirmed, err := client.Account(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("调度开关写入后读取账号失败：%w", err)
	}
	return confirmed, nil
}
