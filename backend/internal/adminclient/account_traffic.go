package adminclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"
)

// AccountTraffic contains only the live gateway counters, never account credentials.
type AccountTraffic struct {
	AccountID       string `json:"account_id"`
	CurrentRequests int64  `json:"current_requests"`
	WaitingRequests int64  `json:"waiting_requests"`
	Tracked         bool   `json:"tracked"`
}

type AccountTrafficSnapshot struct {
	Enabled    bool             `json:"enabled"`
	ObservedAt time.Time        `json:"observed_at"`
	Accounts   []AccountTraffic `json:"accounts"`
}

func (c *Client) AccountTraffic(ctx context.Context) (AccountTrafficSnapshot, error) {
	payload, err := c.request(ctx, http.MethodGet, "/admin/ops/concurrency", nil, nil)
	if err != nil {
		return AccountTrafficSnapshot{}, err
	}
	raw, err := json.Marshal(payload["data"])
	if err != nil {
		return AccountTrafficSnapshot{}, err
	}
	var response struct {
		Enabled   *bool     `json:"enabled"`
		Timestamp time.Time `json:"timestamp"`
		Accounts  map[string]struct {
			ID       json.Number `json:"account_id"`
			Current  *int64      `json:"current_in_use"`
			Waiting  *int64      `json:"waiting_in_queue"`
			Capacity *int64      `json:"max_capacity"`
		} `json:"account"`
	}
	invalid := errors.New("Sub2API 实时并发响应不完整，请检查运维监控版本")
	if json.Unmarshal(raw, &response) != nil || response.Enabled == nil {
		return AccountTrafficSnapshot{}, invalid
	}
	result := AccountTrafficSnapshot{Enabled: *response.Enabled, ObservedAt: response.Timestamp, Accounts: []AccountTraffic{}}
	if !result.Enabled {
		return result, nil
	}
	if response.Accounts == nil || response.Timestamp.IsZero() {
		return AccountTrafficSnapshot{}, invalid
	}
	for key, row := range response.Accounts {
		if !stableID(key) || row.ID.String() != key || row.Current == nil || row.Waiting == nil || row.Capacity == nil || *row.Current < 0 || *row.Waiting < 0 || *row.Capacity < 0 || *row.Current > 9007199254740991 || *row.Waiting > 9007199254740991 {
			return AccountTrafficSnapshot{}, invalid
		}
		result.Accounts = append(result.Accounts, AccountTraffic{AccountID: key, CurrentRequests: *row.Current, WaitingRequests: *row.Waiting, Tracked: *row.Capacity > 0})
	}
	sort.Slice(result.Accounts, func(i, j int) bool { return result.Accounts[i].AccountID < result.Accounts[j].AccountID })
	return result, nil
}
