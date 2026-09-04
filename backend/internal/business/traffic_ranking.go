package business

import (
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	TrafficRankingSortTraffic        = "traffic"
	TrafficRankingSortStability      = "stability"
	TrafficRankingSortSuccessRate    = "success_rate"
	TrafficRankingSortLatency        = "latency"
	trafficRankingLatencySampleLimit = 10000
)

type TrafficRankingQuery struct {
	StartAt   time.Time
	EndAt     time.Time
	GroupName string
	SortBy    string
}

type TrafficRanking struct {
	StartAt             string              `json:"start_at"`
	EndAt               string              `json:"end_at"`
	GroupName           string              `json:"group_name"`
	SortBy              string              `json:"sort_by"`
	Bucket              string              `json:"bucket"`
	TotalRequests       int                 `json:"total_requests"`
	AccountsWithTraffic int                 `json:"accounts_with_traffic"`
	Accounts            []TrafficRankingRow `json:"accounts"`
}

type TrafficRankingRow struct {
	Rank           int      `json:"rank"`
	AccountID      string   `json:"account_id"`
	AccountName    string   `json:"account_name"`
	UpstreamHost   string   `json:"upstream_host"`
	Platform       string   `json:"platform"`
	Groups         []string `json:"groups"`
	Requests       int      `json:"requests"`
	Successful     int      `json:"successful"`
	Failed         int      `json:"failed"`
	TrafficShare   *float64 `json:"traffic_share"`
	SuccessRate    *float64 `json:"success_rate"`
	StabilityScore *float64 `json:"stability_score"`
	AverageLatency *float64 `json:"average_latency_ms"`
	P95Latency     *float64 `json:"p95_latency_ms"`
	ActiveBuckets  int      `json:"active_buckets"`
	TotalBuckets   int      `json:"total_buckets"`
	LatestAt       *string  `json:"latest_at"`
}

type trafficRankingAccumulator struct {
	row           TrafficRankingRow
	latencies     latencySampleMaxHeap
	latencySum    float64
	latencyCount  int
	activeBuckets map[string]struct{}
	latest        time.Time
}

type latencySample struct {
	score uint64
	value float64
}

type latencySampleMaxHeap []latencySample

func (values latencySampleMaxHeap) Len() int { return len(values) }

func (values latencySampleMaxHeap) Less(left, right int) bool {
	return values[left].score > values[right].score
}

func (values latencySampleMaxHeap) Swap(left, right int) {
	values[left], values[right] = values[right], values[left]
}

func (values *latencySampleMaxHeap) Push(value any) {
	*values = append(*values, value.(latencySample))
}

func (values *latencySampleMaxHeap) Pop() any {
	previous := *values
	last := len(previous) - 1
	value := previous[last]
	*values = previous[:last]
	return value
}

func (s *Store) TrafficRanking(ctx context.Context, query TrafficRankingQuery) (TrafficRanking, error) {
	query.StartAt, query.EndAt = query.StartAt.UTC(), query.EndAt.UTC()
	query.GroupName = strings.TrimSpace(query.GroupName)
	query.SortBy = strings.TrimSpace(query.SortBy)
	if query.SortBy == "" {
		query.SortBy = TrafficRankingSortTraffic
	}
	if query.StartAt.IsZero() || query.EndAt.IsZero() || !query.StartAt.Before(query.EndAt) {
		return TrafficRanking{}, errors.New("流量排行时间范围无效")
	}
	if query.EndAt.Sub(query.StartAt) > 30*24*time.Hour {
		return TrafficRanking{}, errors.New("流量排行时间范围不能超过 30 天")
	}
	if !validTrafficRankingSort(query.SortBy) {
		return TrafficRanking{}, errors.New("流量排行排序方式无效")
	}
	accounts, order, err := s.trafficRankingAccounts(ctx)
	if err != nil {
		return TrafficRanking{}, err
	}
	if query.GroupName != "" {
		filtered := order[:0]
		for _, accountID := range order {
			if containsExact(accounts[accountID].row.Groups, query.GroupName) {
				filtered = append(filtered, accountID)
				continue
			}
			delete(accounts, accountID)
		}
		order = filtered
	}
	bucket, bucketDuration := trafficRankingBucket(query.EndAt.Sub(query.StartAt))
	totalBuckets := int(math.Ceil(query.EndAt.Sub(query.StartAt).Hours() / bucketDuration.Hours()))
	for _, account := range accounts {
		account.row.TotalBuckets = totalBuckets
	}
	if err := s.accumulateTrafficRanking(ctx, query, bucketDuration, accounts); err != nil {
		return TrafficRanking{}, err
	}
	result := TrafficRanking{
		StartAt: query.StartAt.Format(time.RFC3339Nano), EndAt: query.EndAt.Format(time.RFC3339Nano),
		GroupName: query.GroupName, SortBy: query.SortBy, Bucket: bucket, Accounts: make([]TrafficRankingRow, 0, len(order)),
	}
	for _, accountID := range order {
		account := accounts[accountID]
		finalizeTrafficRankingAccount(account)
		result.TotalRequests += account.row.Requests
		if account.row.Requests > 0 {
			result.AccountsWithTraffic++
		}
		result.Accounts = append(result.Accounts, account.row)
	}
	for index := range result.Accounts {
		if result.TotalRequests > 0 && result.Accounts[index].Requests > 0 {
			share := roundTrafficMetric(float64(result.Accounts[index].Requests) * 100 / float64(result.TotalRequests))
			result.Accounts[index].TrafficShare = &share
		}
	}
	sortTrafficRanking(result.Accounts, query.SortBy)
	for index := range result.Accounts {
		result.Accounts[index].Rank = index + 1
	}
	return result, nil
}

func (s *Store) trafficRankingAccounts(ctx context.Context) (map[string]*trafficRankingAccumulator, []string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.name,COALESCE(a.upstream_host,''),COALESCE(a.upstream_type,''),
		a.metadata_json,COALESCE(ag.group_name,'') FROM accounts a
		LEFT JOIN account_groups ag ON ag.account_id=a.id
		ORDER BY CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id,ag.group_name`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	accounts := map[string]*trafficRankingAccumulator{}
	order := []string{}
	for rows.Next() {
		var accountID, name, host, upstreamType, metadataRaw, groupName string
		if err := rows.Scan(&accountID, &name, &host, &upstreamType, &metadataRaw, &groupName); err != nil {
			return nil, nil, err
		}
		account := accounts[accountID]
		if account == nil {
			metadata, err := decodeObject(metadataRaw)
			if err != nil {
				return nil, nil, errors.New("账号 " + accountID + " 元数据记录损坏")
			}
			platform, _ := metadata["platform"].(string)
			platform = strings.ToLower(strings.TrimSpace(platform))
			if platform == "" {
				platform = strings.ToLower(strings.TrimSpace(upstreamType))
			}
			account = &trafficRankingAccumulator{
				row:           TrafficRankingRow{AccountID: accountID, AccountName: name, UpstreamHost: host, Platform: platform, Groups: []string{}},
				activeBuckets: map[string]struct{}{},
			}
			accounts[accountID] = account
			order = append(order, accountID)
		}
		if groupName != "" && !containsExact(account.row.Groups, groupName) {
			account.row.Groups = append(account.row.Groups, groupName)
		}
	}
	return accounts, order, rows.Err()
}

func (s *Store) accumulateTrafficRanking(
	ctx context.Context,
	query TrafficRankingQuery,
	bucketDuration time.Duration,
	accounts map[string]*trafficRankingAccumulator,
) error {
	rows, err := s.db.QueryContext(ctx, `WITH ranked AS (
		SELECT request_id,account_id,is_error,first_token_ms,observed_at,payload_json,
			ROW_NUMBER() OVER(PARTITION BY account_id,request_id ORDER BY observed_at,id) AS request_rank
		FROM usage_records WHERE LOWER(source)='traffic' AND observed_at>=? AND observed_at<=?
	)
	SELECT request_id,account_id,is_error,first_token_ms,observed_at,payload_json
	FROM ranked WHERE request_rank=1 ORDER BY observed_at`,
		query.StartAt.Format(time.RFC3339Nano), query.EndAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var requestID, accountID string
		var isError *bool
		var firstToken, observed *string
		var payloadRaw string
		if err := rows.Scan(&requestID, &accountID, &isError, &firstToken, &observed, &payloadRaw); err != nil {
			return err
		}
		account := accounts[accountID]
		if account == nil || observed == nil {
			continue
		}
		observedAt, err := time.Parse(time.RFC3339Nano, *observed)
		if err != nil || observedAt.Before(query.StartAt) || observedAt.After(query.EndAt) {
			continue
		}
		account.row.Requests++
		if isError != nil && !*isError {
			account.row.Successful++
		} else if isError != nil && *isError {
			account.row.Failed++
		}
		var payload map[string]any
		decoder := json.NewDecoder(strings.NewReader(payloadRaw))
		decoder.UseNumber()
		var latency *string
		if decodeErr := decoder.Decode(&payload); decodeErr == nil {
			latency = trafficPayloadLatency(payload)
		}
		if latency == nil {
			latency = firstToken
		}
		if latency != nil {
			value, parseErr := strconv.ParseFloat(strings.TrimSpace(*latency), 64)
			if parseErr == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 {
				account.latencySum += value
				account.latencyCount++
				addTrafficLatencySample(account, accountID, requestID, value)
			}
		}
		bucketAt := observedAt.UTC().Truncate(bucketDuration)
		account.activeBuckets[bucketAt.Format(time.RFC3339)] = struct{}{}
		if account.latest.IsZero() || observedAt.After(account.latest) {
			account.latest = observedAt.UTC()
		}
	}
	return rows.Err()
}

func finalizeTrafficRankingAccount(account *trafficRankingAccumulator) {
	account.row.ActiveBuckets = len(account.activeBuckets)
	if account.row.Requests > 0 {
		rate := roundTrafficMetric(float64(account.row.Successful) * 100 / float64(account.row.Requests))
		stability := roundTrafficMetric(wilsonLowerBound(account.row.Successful, account.row.Requests) * 100)
		account.row.SuccessRate, account.row.StabilityScore = &rate, &stability
	}
	if len(account.latencies) > 0 {
		values := make([]float64, len(account.latencies))
		for index, sample := range account.latencies {
			values[index] = sample.value
		}
		sort.Float64s(values)
		average := roundTrafficMetric(account.latencySum / float64(account.latencyCount))
		p95Index := int(math.Ceil(float64(len(values))*0.95)) - 1
		p95 := roundTrafficMetric(values[max(0, p95Index)])
		account.row.AverageLatency, account.row.P95Latency = &average, &p95
	}
	if !account.latest.IsZero() {
		latest := account.latest.Format(time.RFC3339Nano)
		account.row.LatestAt = &latest
	}
}

func addTrafficLatencySample(account *trafficRankingAccumulator, accountID, requestID string, value float64) {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(accountID))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(requestID))
	sample := latencySample{score: hasher.Sum64(), value: value}
	if len(account.latencies) < trafficRankingLatencySampleLimit {
		heap.Push(&account.latencies, sample)
		return
	}
	if sample.score >= account.latencies[0].score {
		return
	}
	heap.Pop(&account.latencies)
	heap.Push(&account.latencies, sample)
}

func trafficRankingBucket(duration time.Duration) (string, time.Duration) {
	if duration <= 48*time.Hour {
		return "hour", time.Hour
	}
	return "day", 24 * time.Hour
}

func trafficPayloadLatency(payload map[string]any) *string {
	for _, name := range []string{"duration_ms", "latency_ms"} {
		value, found := payload[name]
		if !found || value == nil {
			continue
		}
		text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(toTrafficText(value)), "ms"))
		if text != "" {
			return &text
		}
	}
	return nil
}

func toTrafficText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func wilsonLowerBound(successful, total int) float64 {
	if total <= 0 {
		return 0
	}
	z := 1.959963984540054
	n := float64(total)
	p := float64(successful) / n
	zSquared := z * z
	return (p + zSquared/(2*n) - z*math.Sqrt((p*(1-p)+zSquared/(4*n))/n)) / (1 + zSquared/n)
}

func validTrafficRankingSort(value string) bool {
	return value == TrafficRankingSortTraffic || value == TrafficRankingSortStability ||
		value == TrafficRankingSortSuccessRate || value == TrafficRankingSortLatency
}

func sortTrafficRanking(rows []TrafficRankingRow, sortBy string) {
	sort.SliceStable(rows, func(left, right int) bool {
		first, second := rows[left], rows[right]
		var compared int
		switch sortBy {
		case TrafficRankingSortStability:
			compared = compareOptionalMetric(first.StabilityScore, second.StabilityScore, true)
		case TrafficRankingSortSuccessRate:
			compared = compareOptionalMetric(first.SuccessRate, second.SuccessRate, true)
		case TrafficRankingSortLatency:
			compared = compareOptionalMetric(first.P95Latency, second.P95Latency, false)
		default:
			compared = compareInteger(first.Requests, second.Requests, true)
		}
		if compared != 0 {
			return compared < 0
		}
		if first.Requests != second.Requests {
			return first.Requests > second.Requests
		}
		return stableAccountIDLess(first.AccountID, second.AccountID)
	})
}

func compareOptionalMetric(left, right *float64, descending bool) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}
	if *left == *right {
		return 0
	}
	if descending {
		if *left > *right {
			return -1
		}
		return 1
	}
	if *left < *right {
		return -1
	}
	return 1
}

func compareInteger(left, right int, descending bool) int {
	if left == right {
		return 0
	}
	if (descending && left > right) || (!descending && left < right) {
		return -1
	}
	return 1
}

func stableAccountIDLess(left, right string) bool {
	leftID, leftErr := strconv.ParseUint(left, 10, 64)
	rightID, rightErr := strconv.ParseUint(right, 10, 64)
	if leftErr == nil && rightErr == nil && leftID != rightID {
		return leftID < rightID
	}
	return left < right
}

func containsExact(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func roundTrafficMetric(value float64) float64 {
	return math.Round(value*100) / 100
}
