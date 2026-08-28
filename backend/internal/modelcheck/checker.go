package modelcheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	missingCategory = "__MISSING__"
	otherCategory   = "__OTHER__"
)

var (
	numberPattern = regexp.MustCompile(`^-?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$`)
	thinkPattern  = regexp.MustCompile(`(?is)<think>.*?</think>`)
)

type bundleSender interface {
	Send(context.Context, string, string, string, int) (string, string, error)
}

type targetRequest struct {
	AccountID      string
	AccountName    string
	Model          string
	Rounds         int
	TimeoutSeconds int
}

type bundleResult struct {
	Run           int
	Outputs       map[string]any
	ResponseModel string
	LatencyMS     float64
	Err           error
}

type visibleRequestError struct {
	message string
}

func (err visibleRequestError) Error() string {
	return err.message
}

type scoredProfile struct {
	Fit              float64
	Margin           float64
	Nearest          string
	Coverage         float64
	EvidenceCoverage float64
}

type claudeAudit struct {
	Verdict                     string
	IdentityMatchPercent        *float64
	SameStandardPercent         *float64
	ReplacementExclusionPercent *float64
	Nearest                     string
	CoveragePercent             float64
	EvidenceCoveragePercent     float64
	UsableRounds                int
	RequestedRounds             int
	SuccessfulRequests          int
	TotalRequests               int
	ResponseModels              []string
	FailureReason               string
}

type solScore struct {
	Similarity       map[string]float64
	ProbeCount       int
	Parseable        int
	Evidence         int
	Coverage         float64
	EvidenceCoverage float64
}

type solAudit struct {
	Verdict            string
	Stage              string
	Score              solScore
	SuccessfulRequests int
	TotalRequests      int
	ResponseModels     []string
}

func runClaudeCheck(ctx context.Context, sender bundleSender, profiles map[string]claudeProfile, request targetRequest) (map[string]any, error) {
	standard := inferClaudeStandard(request.Model, profiles)
	profile, found := profiles[standard]
	if !found {
		return nil, errors.New("无法识别 Claude 标准型号，请选择一个标准型号")
	}
	started := time.Now()
	audit := executeClaudeProfile(ctx, sender, profile, request)
	verdict := audit.Verdict
	if audit.SuccessfulRequests == 0 && audit.TotalRequests > 0 {
		verdict = "ERROR"
	}
	if verdict == "MATCH" && len(profile.Group) > 1 {
		verdict = "GROUP_MATCH"
	}
	return map[string]any{
		"account_id": request.AccountID, "account_name": request.AccountName,
		"checker": "claude", "protocol": "anthropic-messages", "claimed_model": request.Model,
		"standard_model": standard, "identity_group": profile.Group, "exact_model_resolved": len(profile.Group) == 1,
		"verdict": verdict, "identity_match_percent": audit.IdentityMatchPercent,
		"same_standard_percent":         audit.SameStandardPercent,
		"replacement_exclusion_percent": audit.ReplacementExclusionPercent,
		"nearest_outside_model":         nullableString(audit.Nearest), "coverage_percent": audit.CoveragePercent,
		"evidence_coverage_percent": audit.EvidenceCoveragePercent,
		"usable_rounds":             audit.UsableRounds, "requested_rounds": audit.RequestedRounds,
		"requests":        map[string]any{"successful": audit.SuccessfulRequests, "total": audit.TotalRequests},
		"response_models": audit.ResponseModels, "elapsed_seconds": round(float64(time.Since(started))/float64(time.Second), 2),
		"error":                 nullableString(audit.FailureReason),
		"scope":                 "behavioral match against a private standard; not model identity proof",
		"credentials_persisted": false,
	}, nil
}

func executeClaudeProfile(ctx context.Context, sender bundleSender, profile claudeProfile, request targetRequest) claudeAudit {
	type task struct {
		run    int
		probes []probe
	}
	tasks := make([]task, 0, request.Rounds*((len(profile.Probes)+11)/12))
	for run := 0; run < request.Rounds; run++ {
		for index := 0; index < len(profile.Probes); index += 12 {
			end := min(index+12, len(profile.Probes))
			tasks = append(tasks, task{run: run, probes: profile.Probes[index:end]})
		}
	}
	results := make([]bundleResult, len(tasks))
	workers := min(6, len(tasks))
	jobs := make(chan int)
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range jobs {
				results[index] = requestBundle(ctx, sender, request, tasks[index].probes, tasks[index].run, "claude")
			}
		}()
	}
	for index := range tasks {
		jobs <- index
	}
	close(jobs)
	group.Wait()

	scored := make([]scoredProfile, request.Rounds)
	for run := 0; run < request.Rounds; run++ {
		answers := map[string]any{}
		for _, result := range results {
			if result.Run != run {
				continue
			}
			for id, value := range result.Outputs {
				answers[id] = value
			}
		}
		scored[run] = scoreClaudeProfile(profile, answers)
	}
	usable := make([]scoredProfile, 0, len(scored))
	for _, score := range scored {
		if score.Coverage >= profile.Thresholds[2] && score.EvidenceCoverage >= profile.Thresholds[3] {
			usable = append(usable, score)
		}
	}
	audit := claudeAudit{Verdict: "INCONCLUSIVE", RequestedRounds: request.Rounds, TotalRequests: len(results)}
	collectRequestSummary(results, &audit.SuccessfulRequests, &audit.ResponseModels)
	audit.FailureReason = requestFailureReason(results)
	if len(usable) == 0 {
		return audit
	}
	aggregate := scoredProfile{}
	nearestMargin := math.Inf(1)
	for _, score := range usable {
		aggregate.Fit += score.Fit
		aggregate.Margin += score.Margin
		aggregate.Coverage += score.Coverage
		aggregate.EvidenceCoverage += score.EvidenceCoverage
		if score.Margin < nearestMargin {
			nearestMargin = score.Margin
			aggregate.Nearest = score.Nearest
		}
	}
	divisor := float64(len(usable))
	aggregate.Fit /= divisor
	aggregate.Margin /= divisor
	aggregate.Coverage /= divisor
	aggregate.EvidenceCoverage /= divisor
	audit.UsableRounds = len(usable)
	audit.Nearest = aggregate.Nearest
	audit.CoveragePercent = round(aggregate.Coverage*100, 1)
	audit.EvidenceCoveragePercent = round(aggregate.EvidenceCoverage*100, 1)
	if aggregate.Fit >= profile.Thresholds[0] && aggregate.Margin >= profile.Thresholds[1] {
		audit.Verdict = "MATCH"
	} else {
		audit.Verdict = "MISMATCH"
	}
	absolute := scoreBand(aggregate.Fit, profile.ScoreBands[0], profile.ScoreBands[1], profile.ScoreBands[2])
	separation := scoreBand(aggregate.Margin, profile.ScoreBands[3], profile.ScoreBands[4], profile.ScoreBands[5])
	identity := round(math.Min(absolute, separation), 1)
	absolute = round(absolute, 1)
	separation = round(separation, 1)
	audit.IdentityMatchPercent = &identity
	audit.SameStandardPercent = &absolute
	audit.ReplacementExclusionPercent = &separation
	return audit
}

func scoreClaudeProfile(profile claudeProfile, answers map[string]any) scoredProfile {
	logs := make([]float64, len(profile.Models))
	parseable, evidence := 0, 0
	for _, current := range profile.Probes {
		value, found := answers[current.ID]
		if found && value != nil {
			parseable++
		}
		category := normalizeValue(current, value)
		weights, found := current.Weights[category]
		if !found || category == missingCategory || category == otherCategory {
			continue
		}
		evidence++
		for index := range logs {
			logs[index] += weights[index]
		}
	}
	member := map[string]bool{}
	for _, model := range profile.Group {
		member[model] = true
	}
	groupLog := math.Inf(-1)
	nearestLog := math.Inf(-1)
	nearest := ""
	for index, model := range profile.Models {
		if member[model] {
			groupLog = math.Max(groupLog, logs[index])
		} else if logs[index] > nearestLog {
			nearestLog, nearest = logs[index], model
		}
	}
	divisor := float64(max(evidence, 1))
	return scoredProfile{
		Fit: groupLog / divisor, Margin: (groupLog - nearestLog) / divisor, Nearest: nearest,
		Coverage:         float64(parseable) / float64(len(profile.Probes)),
		EvidenceCoverage: float64(evidence) / float64(len(profile.Probes)),
	}
}

func scoreBand(value, reject, accept, strong float64) float64 {
	if value >= accept {
		return 97 + 3*math.Min((value-accept)/math.Max(strong-accept, 1e-9), 1)
	}
	width := math.Max(accept-reject, 1e-9)
	if value >= reject {
		return 3 + 94*(value-reject)/width
	}
	return math.Max(0, 3*math.Exp((value-reject)/width))
}

func runSolCheck(ctx context.Context, sender bundleSender, raw solProfile, request targetRequest) (map[string]any, error) {
	quick, err := normalizeSolProbes(raw.Panels["quick"])
	if err != nil {
		return nil, err
	}
	reserve, err := normalizeSolProbes(raw.Panels["reserve"])
	if err != nil {
		return nil, err
	}
	started := time.Now()
	run := time.Now().UnixMilli()
	rows := requestSolPanel(ctx, sender, request, quick, run)
	answers := mergeAnswers(rows)
	score := scoreSol(quick, answers, raw.Models)
	verdict := classifySol(score, raw.Thresholds["quick"], raw.Models)
	stage := "quick"
	if verdict == "INCONCLUSIVE" {
		more := requestSolPanel(ctx, sender, request, reserve, run)
		rows = append(rows, more...)
		for id, value := range mergeAnswers(more) {
			answers[id] = value
		}
		score = scoreSol(append(quick, reserve...), answers, raw.Models)
		verdict = classifySol(score, raw.Thresholds["full"], raw.Models)
		stage = "full"
	}
	successful := 0
	responseModels := []string{}
	collectRequestSummary(rows, &successful, &responseModels)
	failureReason := requestFailureReason(rows)
	if successful == 0 && len(rows) > 0 {
		verdict = "ERROR"
	}
	return map[string]any{
		"account_id": request.AccountID, "account_name": request.AccountName,
		"checker": "sol", "protocol": "openai-responses", "claimed_model": request.Model,
		"verdict": verdict, "stage": stage,
		"similarity_percent": map[string]any{
			"sol":   round(score.Similarity[raw.Models[0]]*100, 1),
			"luna":  round(score.Similarity[raw.Models[1]]*100, 1),
			"terra": round(score.Similarity[raw.Models[2]]*100, 1),
		},
		"coverage":                  map[string]any{"parseable": score.Parseable, "total": score.ProbeCount, "percent": round(score.Coverage*100, 1)},
		"evidence_coverage_percent": round(score.EvidenceCoverage*100, 1),
		"requests":                  map[string]any{"successful": successful, "total": len(rows)},
		"response_models":           responseModels, "elapsed_seconds": round(float64(time.Since(started))/float64(time.Second), 2),
		"error": nullableString(failureReason),
		"scope": "closed-set behavioral similarity; not model identity proof", "credentials_persisted": false,
	}, nil
}

func requestSolPanel(ctx context.Context, sender bundleSender, request targetRequest, probes []probe, run int64) []bundleResult {
	ordered := append([]probe(nil), probes...)
	sort.Slice(ordered, func(left, right int) bool {
		return stableDigest("sol-check", run, ordered[left].ID) < stableDigest("sol-check", run, ordered[right].ID)
	})
	bundles := make([][]probe, 0, (len(ordered)+11)/12)
	for index := 0; index < len(ordered); index += 12 {
		bundles = append(bundles, ordered[index:min(index+12, len(ordered))])
	}
	results := make([]bundleResult, len(bundles))
	var group sync.WaitGroup
	for index := range bundles {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index] = requestBundle(ctx, sender, request, bundles[index], int(run), "sol")
		}(index)
	}
	group.Wait()
	return results
}

func scoreSol(panel []probe, answers map[string]any, models []string) solScore {
	scores := make([]float64, len(models))
	parseable, evidence := 0, 0
	for _, current := range panel {
		value, found := answers[current.ID]
		if found && value != nil {
			parseable++
		}
		category := normalizeValue(current, value)
		weights, found := current.Weights[category]
		if !found || category == missingCategory || category == otherCategory {
			continue
		}
		evidence++
		for index := range scores {
			scores[index] += weights[index]
		}
	}
	peak := slicesMax(scores)
	scale := math.Sqrt(float64(max(evidence, 1)))
	raw, total := make([]float64, len(scores)), 0.0
	for index, value := range scores {
		raw[index] = math.Exp((value - peak) / scale)
		total += raw[index]
	}
	similarity := make(map[string]float64, len(models))
	for index, model := range models {
		similarity[model] = raw[index] / total
	}
	return solScore{
		Similarity: similarity, ProbeCount: len(panel), Parseable: parseable, Evidence: evidence,
		Coverage: float64(parseable) / float64(len(panel)), EvidenceCoverage: float64(evidence) / float64(len(panel)),
	}
}

func classifySol(score solScore, thresholds solThresholds, models []string) string {
	if score.Coverage < thresholds.MinCoverage || score.EvidenceCoverage < thresholds.MinEvidenceCoverage {
		return "INCONCLUSIVE"
	}
	sol, luna, terra := score.Similarity[models[0]], score.Similarity[models[1]], score.Similarity[models[2]]
	if sol >= thresholds.SolAcceptMin {
		return "SOL_CONSISTENT"
	}
	if sol > thresholds.NonSolAcceptMax {
		return "INCONCLUSIVE"
	}
	nonSol := luna + terra
	if nonSol == 0 || math.Max(luna, terra)/nonSol < thresholds.SubtypeAcceptMin {
		return "INCONCLUSIVE"
	}
	if luna > terra {
		return "LUNA_LIKE"
	}
	return "TERRA_LIKE"
}

func requestBundle(ctx context.Context, sender bundleSender, request targetRequest, probes []probe, run int, checker string) bundleResult {
	started := time.Now()
	rendered := make([]string, len(probes))
	mappings := make([]map[string]string, len(probes))
	for index, current := range probes {
		rendered[index], mappings[index] = renderProbe(current, run, checker)
	}
	items := make([]string, len(rendered))
	for index, item := range rendered {
		items[index] = fmt.Sprintf("%d. %s", index+1, item)
	}
	prompt := fmt.Sprintf("Answer all %d items. Return exactly one JSON array with %d strings in the same order. For [NUM], use only a number. For [CHOICE], use only A, B, or C.\n\n%s", len(probes), len(probes), strings.Join(items, "\n"))
	text, responseModel, err := sender.Send(ctx, request.AccountID, request.Model, prompt, request.TimeoutSeconds)
	if err != nil {
		return bundleResult{Run: run, Outputs: emptyOutputs(probes), LatencyMS: elapsedMS(started), Err: safeTransportError(err)}
	}
	parsed := parseJSONArray(text)
	result := bundleResult{Run: run, Outputs: emptyOutputs(probes), ResponseModel: responseModel, LatencyMS: elapsedMS(started)}
	if len(parsed) != len(probes) {
		if looksLikeLegacyFixedGreeting(text) {
			result.Err = errors.New("目标 Sub2API 版本不支持行为检测：账号测试通道未转发检测题目，请升级 Sub2API 服务端")
			return result
		}
		result.Err = errors.New("响应未包含符合要求的 JSON 数组")
		return result
	}
	for index, current := range probes {
		if current.Kind == "numeric" {
			if number, ok := parseNumber(parsed[index]); ok {
				result.Outputs[current.ID] = number
			}
			continue
		}
		letter := strings.ToUpper(strings.TrimSpace(fmt.Sprint(parsed[index])))
		if category, found := mappings[index][letter]; found {
			result.Outputs[current.ID] = category
		}
	}
	return result
}

func looksLikeLegacyFixedGreeting(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "hi" || normalized == "hello" || normalized == "hey" {
		return true
	}
	greetings := []string{
		"hi! what would you like",
		"hello! what would you like",
		"hey! what would you like",
		"how can i help you",
		"how may i help you",
		"ready to help with whatever you need",
	}
	for _, greeting := range greetings {
		if strings.Contains(normalized, greeting) {
			return true
		}
	}
	return false
}

func renderProbe(current probe, run int, checker string) (string, map[string]string) {
	if current.Kind == "numeric" {
		return "[NUM] " + current.Question, map[string]string{}
	}
	indices := []int{0, 1, 2}
	if checker == "claude" {
		sort.Slice(indices, func(left, right int) bool {
			return stableDigest("closed-set-v2", "identity-audit", run, current.ID, indices[left]) < stableDigest("closed-set-v2", "identity-audit", run, current.ID, indices[right])
		})
	} else {
		sort.Slice(indices, func(left, right int) bool {
			return stableDigest("sol-check", run, current.ID, indices[left]) < stableDigest("sol-check", run, current.ID, indices[right])
		})
	}
	letters := []string{"A", "B", "C"}
	options := make([]string, 3)
	mapping := make(map[string]string, 3)
	for position, optionIndex := range indices {
		options[position] = letters[position] + ": " + current.Options[optionIndex]
		mapping[letters[position]] = fmt.Sprintf("o%d", optionIndex)
	}
	stem := current.Question
	if checker == "sol" {
		stem = current.Stem
	}
	return "[CHOICE] " + stem + " " + strings.Join(options, " "), mapping
}

func normalizeValue(current probe, value any) string {
	if value == nil {
		return missingCategory
	}
	if current.Kind == "choice" {
		category, ok := value.(string)
		if !ok {
			return otherCategory
		}
		if _, found := current.Weights[category]; found {
			return category
		}
		return otherCategory
	}
	number, ok := value.(float64)
	if !ok {
		return otherCategory
	}
	bestDistance := math.Inf(1)
	category := otherCategory
	for _, cluster := range current.Clusters {
		allowed := 1e-9
		if current.Tolerance != 0 && current.ToleranceMode == "absolute" {
			allowed = math.Max(0.25, current.Tolerance*0.15)
		} else if current.Tolerance != 0 {
			allowed = math.Max(1e-9, math.Max(math.Max(math.Abs(number), math.Abs(cluster.Center)), 1)*current.Tolerance*0.15)
		}
		distance := math.Abs(number - cluster.Center)
		if distance <= allowed && distance < bestDistance {
			bestDistance, category = distance, cluster.ID
		}
	}
	return category
}

func parseJSONArray(value string) []any {
	cleaned := strings.TrimSpace(thinkPattern.ReplaceAllString(value, ""))
	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimSpace(strings.TrimPrefix(cleaned, "```json"))
		cleaned = strings.TrimSpace(strings.TrimPrefix(cleaned, "```"))
		cleaned = strings.TrimSpace(strings.TrimSuffix(cleaned, "```"))
	}
	start, end := strings.Index(cleaned, "["), strings.LastIndex(cleaned, "]")
	if start < 0 || end < start {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(cleaned[start : end+1]))
	decoder.UseNumber()
	var result []any
	if err := decoder.Decode(&result); err != nil {
		return nil
	}
	return result
}

func parseNumber(value any) (float64, bool) {
	if _, boolean := value.(bool); boolean || value == nil {
		return 0, false
	}
	text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(value), ",", ""), "−", "-"))
	if !numberPattern.MatchString(text) {
		return 0, false
	}
	number, err := strconv.ParseFloat(text, 64)
	return number, err == nil && !math.IsInf(number, 0) && !math.IsNaN(number)
}

func collectRequestSummary(rows []bundleResult, successful *int, responseModels *[]string) {
	models := map[string]bool{}
	for _, row := range rows {
		if row.Err == nil {
			*successful++
		}
		if row.ResponseModel != "" {
			models[row.ResponseModel] = true
		}
	}
	for model := range models {
		*responseModels = append(*responseModels, model)
	}
	sort.Strings(*responseModels)
}

func requestFailureReason(rows []bundleResult) string {
	reasons := map[string]bool{}
	legacyVersionError := ""
	for _, row := range rows {
		if row.Err != nil {
			reason := strings.TrimSpace(row.Err.Error())
			if reason != "" {
				if strings.Contains(reason, "目标 Sub2API 版本不支持行为检测") {
					legacyVersionError = reason
				}
				reasons[reason] = true
			}
		}
	}
	if legacyVersionError != "" {
		return legacyVersionError
	}
	values := make([]string, 0, len(reasons))
	for reason := range reasons {
		values = append(values, reason)
	}
	sort.Strings(values)
	return strings.Join(values, "；")
}

func mergeAnswers(rows []bundleResult) map[string]any {
	answers := map[string]any{}
	for _, row := range rows {
		for id, value := range row.Outputs {
			answers[id] = value
		}
	}
	return answers
}

func emptyOutputs(probes []probe) map[string]any {
	outputs := make(map[string]any, len(probes))
	for _, current := range probes {
		outputs[current.ID] = nil
	}
	return outputs
}

func stableDigest(parts ...any) string {
	values := make([]string, len(parts))
	for index, part := range parts {
		values[index] = fmt.Sprint(part)
	}
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(digest[:])
}

func inferClaudeStandard(model string, profiles map[string]claudeProfile) string {
	if _, found := profiles[model]; found {
		return model
	}
	for index := 1; index < len(model)-1; index++ {
		if model[index] == '-' && model[index-1] >= '0' && model[index-1] <= '9' && model[index+1] >= '0' && model[index+1] <= '9' {
			candidate := model[:index] + "." + model[index+1:]
			if _, found := profiles[candidate]; found {
				return candidate
			}
		}
	}
	return ""
}

func slicesMax(values []float64) float64 {
	result := math.Inf(-1)
	for _, value := range values {
		result = math.Max(result, value)
	}
	return result
}

func safeTransportError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("请求超时")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("请求已取消")
	}
	var visible visibleRequestError
	if errors.As(err, &visible) {
		return errors.New(visible.message)
	}
	return errors.New("无法连接目标接口")
}

func elapsedMS(started time.Time) float64 {
	return round(float64(time.Since(started))/float64(time.Millisecond), 1)
}

func round(value float64, precision int) float64 {
	factor := math.Pow10(precision)
	return math.Round(value*factor) / factor
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
