package adminclient

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// LatencyEnrichmentError preserves usable ops evidence when the usage API fails.
type LatencyEnrichmentError struct{ Cause error }

func (e *LatencyEnrichmentError) Error() string {
	return "真实请求首字采集失败，已保留运维请求记录：" + e.Cause.Error()
}

func (e *LatencyEnrichmentError) Unwrap() error { return e.Cause }

func (c *Client) enrichRequestLatency(ctx context.Context, accountID string, start, end time.Time, limit int, rows []map[string]any) ([]map[string]any, error) {
	pending := make(map[string][]map[string]any)
	for _, row := range rows {
		requestID, _ := row["request_id"].(string)
		requestID = strings.TrimSpace(requestID)
		if fmt.Sprint(row["account_id"]) == accountID && row["kind"] == "success" && requestID != "" && !validFirstToken(row["first_token_ms"]) {
			pending[requestID] = append(pending[requestID], row)
		}
	}
	if len(pending) == 0 {
		return rows, nil
	}
	// The official usage API accepts calendar dates rather than ops timestamps.
	// Request/account identity limits enrichment to the already selected ops rows.
	usage, err := c.fetchEvidence(ctx, "/admin/usage", "真实请求首字记录", map[string]string{
		"account_id": accountID, "start_date": start.UTC().Format(time.DateOnly),
		"end_date": end.UTC().Format(time.DateOnly), "timezone": "UTC",
		"sort_by": "created_at", "sort_order": "desc",
	}, limit, "id", 100)
	if err != nil {
		return rows, &LatencyEnrichmentError{Cause: err}
	}
	for _, item := range usage {
		if fmt.Sprint(item["account_id"]) != accountID || !validFirstToken(item["first_token_ms"]) {
			continue
		}
		requestID, _ := item["request_id"].(string)
		requestID = strings.TrimSpace(requestID)
		for _, row := range pending[requestID] {
			row["first_token_ms"] = item["first_token_ms"]
			row["first_token_source"] = "usage.first_token_ms"
		}
		delete(pending, requestID)
	}
	return rows, nil
}

func validFirstToken(raw any) bool {
	value, err := strconv.ParseFloat(fmt.Sprint(raw), 64)
	return err == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
