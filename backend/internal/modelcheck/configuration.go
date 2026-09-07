package modelcheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const configurationHistoryLimit = 10

type configurationRepository interface {
	LoadModelCheckConfiguration(context.Context) ([]byte, error)
	SaveModelCheckConfiguration(context.Context, []byte, string, string) error
}

type NumericTolerance struct {
	Value float64 `json:"value"`
	Mode  string  `json:"mode"`
}

type NumericClusterDefinition struct {
	ID     string  `json:"id"`
	Center float64 `json:"center"`
}

type ProbeDefinition struct {
	ID        string                     `json:"id"`
	Kind      string                     `json:"kind"`
	Question  string                     `json:"question,omitempty"`
	Stem      string                     `json:"stem,omitempty"`
	Options   []string                   `json:"options,omitempty"`
	Clusters  []NumericClusterDefinition `json:"clusters,omitempty"`
	Tolerance *NumericTolerance          `json:"tolerance,omitempty"`
	Weights   map[string][]float64       `json:"weights"`
}

type ClaudeProfileDefinition struct {
	IdentityGroup   []string          `json:"identity_group"`
	CandidateModels []string          `json:"candidate_models"`
	Thresholds      []float64         `json:"thresholds"`
	ScoreBands      []float64         `json:"score_bands"`
	Probes          []ProbeDefinition `json:"probes"`
}

type SolProfileDefinition struct {
	CandidateModels []string                 `json:"candidate_models"`
	Quick           []ProbeDefinition        `json:"quick"`
	Reserve         []ProbeDefinition        `json:"reserve"`
	Thresholds      map[string]solThresholds `json:"thresholds"`
}

type ProfilePayload struct {
	ClaudeProfiles map[string]ClaudeProfileDefinition `json:"claude_profiles"`
	SolProfile     SolProfileDefinition               `json:"sol_profile"`
}

type ConfigurationVersion struct {
	ID          string         `json:"id"`
	Status      string         `json:"status"`
	Note        string         `json:"note"`
	Fingerprint string         `json:"fingerprint"`
	CreatedAt   string         `json:"created_at"`
	CreatedBy   string         `json:"created_by"`
	PublishedAt *string        `json:"published_at"`
	Payload     ProfilePayload `json:"payload"`
}

type ConfigurationVersionSummary struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	Note           string  `json:"note"`
	Fingerprint    string  `json:"fingerprint"`
	CreatedAt      string  `json:"created_at"`
	CreatedBy      string  `json:"created_by"`
	PublishedAt    *string `json:"published_at"`
	ClaudeProfiles int     `json:"claude_profiles"`
	ProbeCount     int     `json:"probe_count"`
}

type ConfigurationView struct {
	Active  ConfigurationVersion          `json:"active"`
	Draft   *ConfigurationVersion         `json:"draft"`
	History []ConfigurationVersionSummary `json:"history"`
}

type SaveDraftRequest struct {
	ExpectedFingerprint string         `json:"expected_fingerprint"`
	Note                string         `json:"note"`
	Payload             ProfilePayload `json:"payload"`
}

type PublishRequest struct {
	ExpectedFingerprint string `json:"expected_fingerprint"`
}

type RestoreRequest struct {
	VersionID           string `json:"version_id"`
	ExpectedFingerprint string `json:"expected_fingerprint"`
	Note                string `json:"note"`
}

type configurationState struct {
	Active  ConfigurationVersion   `json:"active"`
	Draft   *ConfigurationVersion  `json:"draft,omitempty"`
	History []ConfigurationVersion `json:"history"`
}

func newBuiltinConfiguration(claude map[string]claudeProfile, sol solProfile) configurationState {
	payload := definitionsFromProfiles(claude, sol)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	fingerprint := payloadFingerprint(payload)
	return configurationState{Active: ConfigurationVersion{
		ID: "builtin", Status: "published", Note: "系统内置检测画像", Fingerprint: fingerprint,
		CreatedAt: now, CreatedBy: "system", PublishedAt: &now, Payload: payload,
	}, History: []ConfigurationVersion{}}
}

func decodeConfigurationState(raw []byte) (configurationState, error) {
	var state configurationState
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return configurationState{}, fmt.Errorf("模型检测画像配置格式无效: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return configurationState{}, errors.New("模型检测画像配置包含多余内容")
	}
	if state.Active.Status != "published" || state.Active.ID == "" {
		return configurationState{}, errors.New("模型检测画像配置缺少已发布版本")
	}
	if err := validateVersion(&state.Active); err != nil {
		return configurationState{}, fmt.Errorf("已发布画像无效: %w", err)
	}
	if state.Draft != nil {
		if state.Draft.Status != "draft" {
			return configurationState{}, errors.New("模型检测画像草稿状态无效")
		}
		if err := validateVersion(state.Draft); err != nil {
			return configurationState{}, fmt.Errorf("画像草稿无效: %w", err)
		}
	}
	if len(state.History) > configurationHistoryLimit {
		return configurationState{}, errors.New("模型检测画像历史版本过多")
	}
	for index := range state.History {
		if state.History[index].Status != "archived" {
			return configurationState{}, errors.New("模型检测画像历史状态无效")
		}
		if err := validateVersion(&state.History[index]); err != nil {
			return configurationState{}, fmt.Errorf("历史画像无效: %w", err)
		}
	}
	return state, nil
}

func validateVersion(version *ConfigurationVersion) error {
	if version.Fingerprint != payloadFingerprint(version.Payload) {
		return errors.New("画像内容指纹不匹配")
	}
	_, _, err := profilesFromDefinitions(version.Payload)
	return err
}

func (s *Service) Configuration() ConfigurationView {
	s.profilesMu.RLock()
	defer s.profilesMu.RUnlock()
	return configurationView(s.configuration)
}

func (s *Service) SaveDraft(ctx context.Context, request SaveDraftRequest, actor string) (ConfigurationView, error) {
	if s.profileRepository == nil {
		return ConfigurationView{}, errors.New("模型检测画像存储尚未就绪")
	}
	actor = normalizedActor(actor)
	request.Note = strings.TrimSpace(request.Note)
	if utf8.RuneCountInString(request.Note) > 200 {
		return ConfigurationView{}, errors.New("版本说明不能超过 200 个字符")
	}
	if _, _, err := profilesFromDefinitions(request.Payload); err != nil {
		return ConfigurationView{}, err
	}
	s.profilesMu.Lock()
	defer s.profilesMu.Unlock()
	expected := s.configuration.Active.Fingerprint
	if s.configuration.Draft != nil {
		expected = s.configuration.Draft.Fingerprint
	}
	if strings.TrimSpace(request.ExpectedFingerprint) != expected {
		return ConfigurationView{}, errors.New("画像配置已变化，请刷新后重新编辑")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := ""
	createdAt := now
	if s.configuration.Draft != nil {
		id = s.configuration.Draft.ID
		createdAt = s.configuration.Draft.CreatedAt
	} else {
		var err error
		id, err = randomConfigurationID()
		if err != nil {
			return ConfigurationView{}, err
		}
	}
	nextState := s.configuration
	nextState.Draft = &ConfigurationVersion{
		ID: id, Status: "draft", Note: request.Note, Fingerprint: payloadFingerprint(request.Payload),
		CreatedAt: createdAt, CreatedBy: actor, Payload: request.Payload,
	}
	if err := s.persistConfiguration(ctx, nextState, actor, "draft.saved"); err != nil {
		return ConfigurationView{}, err
	}
	s.configuration = nextState
	return configurationView(s.configuration), nil
}

func (s *Service) PublishDraft(ctx context.Context, request PublishRequest, actor string) (ConfigurationView, error) {
	if s.profileRepository == nil {
		return ConfigurationView{}, errors.New("模型检测画像存储尚未就绪")
	}
	s.profilesMu.Lock()
	defer s.profilesMu.Unlock()
	if s.configuration.Draft == nil {
		return ConfigurationView{}, errors.New("没有可发布的画像草稿")
	}
	if strings.TrimSpace(request.ExpectedFingerprint) != s.configuration.Draft.Fingerprint {
		return ConfigurationView{}, errors.New("画像草稿已变化，请刷新后再发布")
	}
	nextState := s.configuration
	previous := nextState.Active
	previous.Status = "archived"
	nextState.History = append([]ConfigurationVersion{previous}, nextState.History...)
	if len(nextState.History) > configurationHistoryLimit {
		nextState.History = nextState.History[:configurationHistoryLimit]
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	next := *nextState.Draft
	next.Status = "published"
	next.PublishedAt = &now
	next.CreatedBy = normalizedActor(actor)
	nextState.Active = next
	nextState.Draft = nil
	claude, sol, err := profilesFromDefinitions(next.Payload)
	if err != nil {
		return ConfigurationView{}, err
	}
	if err := s.persistConfiguration(ctx, nextState, normalizedActor(actor), "version.published"); err != nil {
		return ConfigurationView{}, err
	}
	s.configuration = nextState
	s.claudeProfiles, s.solProfile = claude, sol
	return configurationView(s.configuration), nil
}

func (s *Service) DiscardDraft(ctx context.Context, request PublishRequest, actor string) (ConfigurationView, error) {
	if s.profileRepository == nil {
		return ConfigurationView{}, errors.New("模型检测画像存储尚未就绪")
	}
	s.profilesMu.Lock()
	defer s.profilesMu.Unlock()
	if s.configuration.Draft == nil {
		return configurationView(s.configuration), nil
	}
	if strings.TrimSpace(request.ExpectedFingerprint) != s.configuration.Draft.Fingerprint {
		return ConfigurationView{}, errors.New("画像草稿已变化，请刷新后再删除")
	}
	nextState := s.configuration
	nextState.Draft = nil
	if err := s.persistConfiguration(ctx, nextState, normalizedActor(actor), "draft.discarded"); err != nil {
		return ConfigurationView{}, err
	}
	s.configuration = nextState
	return configurationView(s.configuration), nil
}

func (s *Service) RestoreVersion(ctx context.Context, request RestoreRequest, actor string) (ConfigurationView, error) {
	if s.profileRepository == nil {
		return ConfigurationView{}, errors.New("模型检测画像存储尚未就绪")
	}
	s.profilesMu.Lock()
	defer s.profilesMu.Unlock()
	expected := s.configuration.Active.Fingerprint
	if s.configuration.Draft != nil {
		expected = s.configuration.Draft.Fingerprint
	}
	if strings.TrimSpace(request.ExpectedFingerprint) != expected {
		return ConfigurationView{}, errors.New("画像配置已变化，请刷新后重试")
	}
	var source *ConfigurationVersion
	if request.VersionID == s.configuration.Active.ID {
		source = &s.configuration.Active
	} else {
		for index := range s.configuration.History {
			if s.configuration.History[index].ID == request.VersionID {
				source = &s.configuration.History[index]
				break
			}
		}
	}
	if source == nil {
		return ConfigurationView{}, errors.New("指定的画像版本不存在")
	}
	id, err := randomConfigurationID()
	if err != nil {
		return ConfigurationView{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	note := strings.TrimSpace(request.Note)
	if note == "" {
		note = "恢复自版本 " + source.ID
	}
	if utf8.RuneCountInString(note) > 200 {
		return ConfigurationView{}, errors.New("版本说明不能超过 200 个字符")
	}
	nextState := s.configuration
	nextState.Draft = &ConfigurationVersion{
		ID: id, Status: "draft", Note: note, Fingerprint: source.Fingerprint,
		CreatedAt: now, CreatedBy: normalizedActor(actor), Payload: source.Payload,
	}
	if err := s.persistConfiguration(ctx, nextState, normalizedActor(actor), "version.restored"); err != nil {
		return ConfigurationView{}, err
	}
	s.configuration = nextState
	return configurationView(s.configuration), nil
}

func (s *Service) persistConfiguration(ctx context.Context, state configurationState, actor, action string) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return errors.New("模型检测画像配置编码失败")
	}
	if err := s.profileRepository.SaveModelCheckConfiguration(ctx, raw, actor, action); err != nil {
		return fmt.Errorf("模型检测画像配置保存失败: %w", err)
	}
	return nil
}

func configurationView(state configurationState) ConfigurationView {
	history := make([]ConfigurationVersionSummary, len(state.History))
	for index := range state.History {
		history[index] = versionSummary(state.History[index])
	}
	active := cloneConfigurationVersion(state.Active)
	var draft *ConfigurationVersion
	if state.Draft != nil {
		cloned := cloneConfigurationVersion(*state.Draft)
		draft = &cloned
	}
	return ConfigurationView{Active: active, Draft: draft, History: history}
}

func cloneConfigurationVersion(version ConfigurationVersion) ConfigurationVersion {
	raw, err := json.Marshal(version)
	if err != nil {
		return ConfigurationVersion{}
	}
	var cloned ConfigurationVersion
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return ConfigurationVersion{}
	}
	return cloned
}

func versionSummary(version ConfigurationVersion) ConfigurationVersionSummary {
	return ConfigurationVersionSummary{
		ID: version.ID, Status: version.Status, Note: version.Note, Fingerprint: version.Fingerprint,
		CreatedAt: version.CreatedAt, CreatedBy: version.CreatedBy, PublishedAt: version.PublishedAt,
		ClaudeProfiles: len(version.Payload.ClaudeProfiles), ProbeCount: profileProbeCount(version.Payload),
	}
}

func profileProbeCount(payload ProfilePayload) int {
	total := len(payload.SolProfile.Quick) + len(payload.SolProfile.Reserve)
	for _, profile := range payload.ClaudeProfiles {
		total += len(profile.Probes)
	}
	return total
}

func payloadFingerprint(payload ProfilePayload) string {
	raw, _ := json.Marshal(payload)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func randomConfigurationID() (string, error) {
	id, err := randomTaskID()
	if err != nil {
		return "", err
	}
	return "profile-" + strings.TrimPrefix(id, "model-check-"), nil
}

func normalizedActor(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "console"
	}
	return actor
}

func definitionsFromProfiles(claude map[string]claudeProfile, sol solProfile) ProfilePayload {
	definitions := make(map[string]ClaudeProfileDefinition, len(claude))
	for name, profile := range claude {
		definition := ClaudeProfileDefinition{
			IdentityGroup: append([]string(nil), profile.Group...), CandidateModels: append([]string(nil), profile.Models...),
			Thresholds: append([]float64(nil), profile.Thresholds[:]...), ScoreBands: append([]float64(nil), profile.ScoreBands[:]...),
			Probes: probeDefinitions(profile.Probes),
		}
		definitions[name] = definition
	}
	quick, _ := normalizeSolProbes(sol.Panels["quick"])
	reserve, _ := normalizeSolProbes(sol.Panels["reserve"])
	return ProfilePayload{ClaudeProfiles: definitions, SolProfile: SolProfileDefinition{
		CandidateModels: append([]string(nil), sol.Models...), Quick: probeDefinitions(quick),
		Reserve: probeDefinitions(reserve), Thresholds: sol.Thresholds,
	}}
}

func probeDefinitions(probes []probe) []ProbeDefinition {
	result := make([]ProbeDefinition, len(probes))
	for index, current := range probes {
		definition := ProbeDefinition{
			ID: current.ID, Kind: current.Kind, Question: current.Question, Stem: current.Stem,
			Options: append([]string(nil), current.Options...), Clusters: clusterDefinitions(current.Clusters),
			Weights: current.Weights,
		}
		if current.Kind == "numeric" {
			definition.Tolerance = &NumericTolerance{Value: current.Tolerance, Mode: current.ToleranceMode}
		}
		result[index] = definition
	}
	return result
}

func clusterDefinitions(clusters []numericCluster) []NumericClusterDefinition {
	result := make([]NumericClusterDefinition, len(clusters))
	for index, cluster := range clusters {
		result[index] = NumericClusterDefinition{ID: cluster.ID, Center: cluster.Center}
	}
	return result
}

func profilesFromDefinitions(payload ProfilePayload) (map[string]claudeProfile, solProfile, error) {
	if len(payload.ClaudeProfiles) == 0 || len(payload.ClaudeProfiles) > 100 {
		return nil, solProfile{}, errors.New("Claude 画像数量必须在 1 到 100 之间")
	}
	if profileProbeCount(payload) > 5000 {
		return nil, solProfile{}, errors.New("检测题目总数不能超过 5000")
	}
	claude := make(map[string]claudeProfile, len(payload.ClaudeProfiles))
	names := make([]string, 0, len(payload.ClaudeProfiles))
	for name := range payload.ClaudeProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		definition := payload.ClaudeProfiles[name]
		if strings.TrimSpace(name) != name || name == "" || utf8.RuneCountInString(name) > 256 {
			return nil, solProfile{}, errors.New("Claude 标准型号名称无效")
		}
		raw := rawClaudeProfile{
			Group: definition.IdentityGroup, Models: definition.CandidateModels,
			Thresholds: definition.Thresholds, ScoreBands: definition.ScoreBands,
		}
		if !validClaudeThresholds(definition.Thresholds, definition.ScoreBands) {
			return nil, solProfile{}, fmt.Errorf("Claude 标准 %s 的判定阈值无效", name)
		}
		var err error
		raw.Probes, err = rawClaudeProbes(definition.Probes, len(definition.CandidateModels))
		if err != nil {
			return nil, solProfile{}, fmt.Errorf("Claude 标准 %s 无效: %w", name, err)
		}
		profile, err := normalizeClaudeProfile(raw)
		if err != nil {
			return nil, solProfile{}, fmt.Errorf("Claude 标准 %s 无效: %w", name, err)
		}
		if err := validateProfileStrings(profile.Group, profile.Models, profile.Probes); err != nil {
			return nil, solProfile{}, fmt.Errorf("Claude 标准 %s 无效: %w", name, err)
		}
		claude[name] = profile
	}
	solRaw, err := solFromDefinition(payload.SolProfile)
	if err != nil {
		return nil, solProfile{}, err
	}
	return claude, solRaw, nil
}

func rawClaudeProbes(definitions []ProbeDefinition, modelCount int) ([]rawClaudeProbe, error) {
	if len(definitions) == 0 || len(definitions) > 500 {
		return nil, errors.New("每个画像的题目数必须在 1 到 500 之间")
	}
	if err := validateProbeDefinitions(definitions, modelCount, false); err != nil {
		return nil, err
	}
	result := make([]rawClaudeProbe, len(definitions))
	for index, definition := range definitions {
		clusters, tolerance, err := encodeProbeNumericFields(definition)
		if err != nil {
			return nil, err
		}
		result[index] = rawClaudeProbe{
			ID: definition.ID, Kind: definition.Kind, Question: definition.Question,
			Options: definition.Options, Clusters: clusters, Tolerance: tolerance, Weights: definition.Weights,
		}
	}
	return result, nil
}

func solFromDefinition(definition SolProfileDefinition) (solProfile, error) {
	if len(definition.CandidateModels) != 3 || !uniqueNonemptyStrings(definition.CandidateModels, 256) {
		return solProfile{}, errors.New("Sol 画像必须包含三个不同的候选模型")
	}
	if len(definition.Quick) == 0 || len(definition.Quick) > 500 || len(definition.Reserve) == 0 || len(definition.Reserve) > 500 {
		return solProfile{}, errors.New("Sol 快速题库和补充题库都必须包含 1 到 500 道题")
	}
	combined := append(append([]ProbeDefinition(nil), definition.Quick...), definition.Reserve...)
	if err := validateProbeDefinitions(combined, 3, true); err != nil {
		return solProfile{}, fmt.Errorf("Sol 画像无效: %w", err)
	}
	for _, stage := range []string{"quick", "full"} {
		threshold, found := definition.Thresholds[stage]
		if !found || !validSolThreshold(threshold) {
			return solProfile{}, fmt.Errorf("Sol %s 判定阈值无效", stage)
		}
	}
	if len(definition.Thresholds) != 2 {
		return solProfile{}, errors.New("Sol 判定阈值只能包含 quick 和 full")
	}
	quick, err := rawSolProbes(definition.Quick)
	if err != nil {
		return solProfile{}, err
	}
	reserve, err := rawSolProbes(definition.Reserve)
	if err != nil {
		return solProfile{}, err
	}
	return solProfile{Models: definition.CandidateModels, Panels: map[string][]rawSolProbe{
		"quick": quick, "reserve": reserve,
	}, Thresholds: definition.Thresholds}, nil
}

func rawSolProbes(definitions []ProbeDefinition) ([]rawSolProbe, error) {
	result := make([]rawSolProbe, len(definitions))
	for index, definition := range definitions {
		_, tolerance, err := encodeProbeNumericFields(definition)
		if err != nil {
			return nil, err
		}
		result[index] = rawSolProbe{
			ID: definition.ID, Kind: definition.Kind, Question: definition.Question, Stem: definition.Stem,
			Options: definition.Options, Clusters: numericClusters(definition.Clusters), Tolerance: tolerance, Weights: definition.Weights,
		}
	}
	return result, nil
}

func numericClusters(clusters []NumericClusterDefinition) []numericCluster {
	result := make([]numericCluster, len(clusters))
	for index, cluster := range clusters {
		result[index] = numericCluster{ID: cluster.ID, Center: cluster.Center}
	}
	return result
}

func encodeProbeNumericFields(definition ProbeDefinition) ([]json.RawMessage, []json.RawMessage, error) {
	if definition.Kind != "numeric" {
		return nil, nil, nil
	}
	clusters := make([]json.RawMessage, len(definition.Clusters))
	for index, cluster := range definition.Clusters {
		encoded, err := json.Marshal([]any{cluster.ID, cluster.Center})
		if err != nil {
			return nil, nil, err
		}
		clusters[index] = encoded
	}
	if definition.Tolerance == nil {
		return nil, nil, errors.New("数值题缺少容差")
	}
	value, _ := json.Marshal(definition.Tolerance.Value)
	mode, _ := json.Marshal(definition.Tolerance.Mode)
	return clusters, []json.RawMessage{value, mode}, nil
}

func validateProbeDefinitions(definitions []ProbeDefinition, modelCount int, sol bool) error {
	seen := map[string]bool{}
	for _, definition := range definitions {
		if definition.ID == "" || utf8.RuneCountInString(definition.ID) > 128 || seen[definition.ID] {
			return errors.New("题目标识不能为空、重复或超过 128 个字符")
		}
		seen[definition.ID] = true
		if definition.Kind != "numeric" && definition.Kind != "choice" {
			return fmt.Errorf("题目 %s 的题型无效", definition.ID)
		}
		prompt := definition.Question
		if sol && definition.Kind == "choice" {
			prompt = definition.Stem
		}
		if strings.TrimSpace(prompt) == "" || utf8.RuneCountInString(prompt) > 2000 {
			return fmt.Errorf("题目 %s 的题干为空或超过 2000 个字符", definition.ID)
		}
		if definition.Kind == "choice" {
			if len(definition.Options) != 3 || !uniqueNonemptyStrings(definition.Options, 1000) {
				return fmt.Errorf("选择题 %s 必须包含三个不同且非空的选项", definition.ID)
			}
		} else {
			if len(definition.Clusters) == 0 || len(definition.Clusters) > 100 || definition.Tolerance == nil ||
				(definition.Tolerance.Mode != "absolute" && definition.Tolerance.Mode != "relative") ||
				definition.Tolerance.Value < 0 || !finite(definition.Tolerance.Value) {
				return fmt.Errorf("数值题 %s 的聚类或容差无效", definition.ID)
			}
			clusterIDs := make([]string, len(definition.Clusters))
			for index, cluster := range definition.Clusters {
				clusterIDs[index] = cluster.ID
				if !finite(cluster.Center) {
					return fmt.Errorf("数值题 %s 的聚类中心无效", definition.ID)
				}
			}
			if !uniqueNonemptyStrings(clusterIDs, 128) {
				return fmt.Errorf("数值题 %s 的聚类标识无效", definition.ID)
			}
		}
		if err := validateWeights(definition.Weights, modelCount); err != nil {
			return fmt.Errorf("题目 %s: %w", definition.ID, err)
		}
		expectedCategories := map[string]bool{}
		if definition.Kind == "choice" {
			expectedCategories = map[string]bool{"o0": true, "o1": true, "o2": true}
		} else {
			for _, cluster := range definition.Clusters {
				expectedCategories[cluster.ID] = true
			}
		}
		if len(definition.Weights) != len(expectedCategories) {
			return fmt.Errorf("题目 %s 的评分权重必须覆盖全部答案类别", definition.ID)
		}
		for category := range definition.Weights {
			if !expectedCategories[category] {
				return fmt.Errorf("题目 %s 包含未知评分类别 %s", definition.ID, category)
			}
		}
		for _, weights := range definition.Weights {
			for _, weight := range weights {
				if !finite(weight) {
					return fmt.Errorf("题目 %s 的评分权重必须是有限数值", definition.ID)
				}
			}
		}
	}
	return nil
}

func validateProfileStrings(group, models []string, probes []probe) error {
	if !uniqueNonemptyStrings(models, 256) || !uniqueNonemptyStrings(group, 256) {
		return errors.New("身份组和候选模型必须使用不同且非空的型号")
	}
	known := map[string]bool{}
	for _, model := range models {
		known[model] = true
	}
	for _, model := range group {
		if !known[model] {
			return errors.New("身份组型号必须包含在候选模型中")
		}
	}
	for _, current := range probes {
		for _, weights := range current.Weights {
			for _, weight := range weights {
				if !finite(weight) {
					return errors.New("评分权重必须是有限数值")
				}
			}
		}
	}
	return nil
}

func uniqueNonemptyStrings(values []string, maxLength int) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) != value || value == "" || utf8.RuneCountInString(value) > maxLength || seen[value] {
			return false
		}
		seen[value] = true
	}
	return len(values) > 0
}

func validSolThreshold(value solThresholds) bool {
	values := []float64{value.SolAcceptMin, value.NonSolAcceptMax, value.SubtypeAcceptMin, value.MinCoverage, value.MinEvidenceCoverage}
	for _, current := range values {
		if !finite(current) || current < 0 || current > 1 {
			return false
		}
	}
	return value.NonSolAcceptMax <= value.SolAcceptMin
}

func validClaudeThresholds(thresholds, scoreBands []float64) bool {
	if len(thresholds) != 4 || len(scoreBands) != 6 {
		return false
	}
	for _, value := range append(append([]float64(nil), thresholds...), scoreBands...) {
		if !finite(value) {
			return false
		}
	}
	if thresholds[2] < 0 || thresholds[2] > 1 || thresholds[3] < 0 || thresholds[3] > 1 {
		return false
	}
	return scoreBands[0] < scoreBands[1] && scoreBands[1] < scoreBands[2] &&
		scoreBands[3] < scoreBands[4] && scoreBands[4] < scoreBands[5]
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
