package modelcheck

import (
	"bytes"
	"compress/zlib"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
)

//go:embed assets/claude-profiles.dat
var claudeProfilesData []byte

//go:embed assets/sol-profile.json
var solProfileData []byte

type claudeProfileEnvelope struct {
	Version  int                         `json:"v"`
	Profiles map[string]rawClaudeProfile `json:"d"`
}

type rawClaudeProfile struct {
	Group      []string         `json:"g"`
	Models     []string         `json:"m"`
	Thresholds []float64        `json:"h"`
	ScoreBands []float64        `json:"s"`
	Probes     []rawClaudeProbe `json:"p"`
}

type rawClaudeProbe struct {
	ID        string               `json:"i"`
	Kind      string               `json:"k"`
	Question  string               `json:"q"`
	Options   []string             `json:"o"`
	Clusters  []json.RawMessage    `json:"c"`
	Tolerance []json.RawMessage    `json:"x"`
	Weights   map[string][]float64 `json:"w"`
}

type claudeProfile struct {
	Group      []string
	Models     []string
	Thresholds [4]float64
	ScoreBands [6]float64
	Probes     []probe
}

type solProfile struct {
	Models     []string                 `json:"models"`
	Panels     map[string][]rawSolProbe `json:"panels"`
	Thresholds map[string]solThresholds `json:"thresholds"`
}

type rawSolProbe struct {
	ID        string               `json:"id"`
	Kind      string               `json:"kind"`
	Question  string               `json:"question"`
	Stem      string               `json:"stem"`
	Options   []string             `json:"options"`
	Clusters  []numericCluster     `json:"clusters"`
	Tolerance []json.RawMessage    `json:"numeric_tolerance"`
	Weights   map[string][]float64 `json:"weights"`
}

type solThresholds struct {
	SolAcceptMin        float64 `json:"sol_accept_min"`
	NonSolAcceptMax     float64 `json:"non_sol_accept_max"`
	SubtypeAcceptMin    float64 `json:"subtype_accept_min"`
	MinCoverage         float64 `json:"min_coverage"`
	MinEvidenceCoverage float64 `json:"min_evidence_coverage"`
}

type numericCluster struct {
	ID     string  `json:"id"`
	Center float64 `json:"center"`
}

type probe struct {
	ID            string
	Kind          string
	Question      string
	Stem          string
	Options       []string
	Clusters      []numericCluster
	Tolerance     float64
	ToleranceMode string
	Weights       map[string][]float64
}

func loadClaudeProfiles() (map[string]claudeProfile, error) {
	reader, err := zlib.NewReader(bytes.NewReader(claudeProfilesData))
	if err != nil {
		return nil, fmt.Errorf("Claude 检测画像无法解压：%w", err)
	}
	decompressed, err := io.ReadAll(io.LimitReader(reader, 4<<20))
	closeErr := reader.Close()
	if err != nil {
		return nil, fmt.Errorf("Claude 检测画像无法读取：%w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("Claude 检测画像校验失败：%w", closeErr)
	}
	var envelope claudeProfileEnvelope
	if err := json.Unmarshal(decompressed, &envelope); err != nil {
		return nil, fmt.Errorf("Claude 检测画像格式无效：%w", err)
	}
	if envelope.Version != 1 || len(envelope.Profiles) == 0 {
		return nil, fmt.Errorf("Claude 检测画像版本不受支持")
	}
	profiles := make(map[string]claudeProfile, len(envelope.Profiles))
	for name, raw := range envelope.Profiles {
		profile, err := normalizeClaudeProfile(raw)
		if err != nil {
			return nil, fmt.Errorf("Claude 标准 %s 无效：%w", name, err)
		}
		profiles[name] = profile
	}
	return profiles, nil
}

func normalizeClaudeProfile(raw rawClaudeProfile) (claudeProfile, error) {
	if len(raw.Models) < 2 || len(raw.Group) == 0 || len(raw.Thresholds) != 4 || len(raw.ScoreBands) != 6 || len(raw.Probes) == 0 {
		return claudeProfile{}, fmt.Errorf("画像字段不完整")
	}
	profile := claudeProfile{Group: raw.Group, Models: raw.Models, Probes: make([]probe, 0, len(raw.Probes))}
	copy(profile.Thresholds[:], raw.Thresholds)
	copy(profile.ScoreBands[:], raw.ScoreBands)
	for _, rawProbe := range raw.Probes {
		current := probe{
			ID: rawProbe.ID, Kind: rawProbe.Kind, Question: rawProbe.Question,
			Options: rawProbe.Options, Weights: rawProbe.Weights,
		}
		if current.ID == "" || current.Question == "" || (current.Kind != "choice" && current.Kind != "numeric") {
			return claudeProfile{}, fmt.Errorf("探针字段无效")
		}
		if current.Kind == "numeric" {
			clusters, err := decodeClaudeClusters(rawProbe.Clusters)
			if err != nil {
				return claudeProfile{}, err
			}
			current.Clusters = clusters
			current.Tolerance, current.ToleranceMode, err = decodeTolerance(rawProbe.Tolerance)
			if err != nil {
				return claudeProfile{}, err
			}
		} else if len(current.Options) != 3 {
			return claudeProfile{}, fmt.Errorf("选择探针必须包含三个选项")
		}
		if err := validateWeights(current.Weights, len(raw.Models)); err != nil {
			return claudeProfile{}, err
		}
		profile.Probes = append(profile.Probes, current)
	}
	return profile, nil
}

func decodeClaudeClusters(values []json.RawMessage) ([]numericCluster, error) {
	clusters := make([]numericCluster, 0, len(values))
	for _, value := range values {
		var pair []json.RawMessage
		if err := json.Unmarshal(value, &pair); err != nil || len(pair) != 2 {
			return nil, fmt.Errorf("数值聚类格式无效")
		}
		var cluster numericCluster
		if err := json.Unmarshal(pair[0], &cluster.ID); err != nil || cluster.ID == "" {
			return nil, fmt.Errorf("数值聚类标识无效")
		}
		if err := json.Unmarshal(pair[1], &cluster.Center); err != nil {
			return nil, fmt.Errorf("数值聚类中心无效")
		}
		clusters = append(clusters, cluster)
	}
	if len(clusters) == 0 {
		return nil, fmt.Errorf("数值探针缺少聚类")
	}
	return clusters, nil
}

func decodeTolerance(values []json.RawMessage) (float64, string, error) {
	if len(values) != 2 {
		return 0, "", fmt.Errorf("数值容差格式无效")
	}
	var tolerance float64
	var mode string
	if err := json.Unmarshal(values[0], &tolerance); err != nil || tolerance < 0 {
		return 0, "", fmt.Errorf("数值容差无效")
	}
	if err := json.Unmarshal(values[1], &mode); err != nil || (mode != "absolute" && mode != "relative") {
		return 0, "", fmt.Errorf("数值容差模式无效")
	}
	return tolerance, mode, nil
}

func loadSolProfile() (solProfile, error) {
	var raw solProfile
	if err := json.Unmarshal(solProfileData, &raw); err != nil {
		return solProfile{}, fmt.Errorf("Sol 检测画像格式无效：%w", err)
	}
	if len(raw.Models) != 3 || len(raw.Panels["quick"]) == 0 || len(raw.Panels["reserve"]) == 0 {
		return solProfile{}, fmt.Errorf("Sol 检测画像字段不完整")
	}
	return raw, nil
}

func normalizeSolProbes(values []rawSolProbe) ([]probe, error) {
	probes := make([]probe, 0, len(values))
	for _, raw := range values {
		current := probe{
			ID: raw.ID, Kind: raw.Kind, Question: raw.Question, Stem: raw.Stem,
			Options: raw.Options, Clusters: raw.Clusters, Weights: raw.Weights,
		}
		if current.ID == "" || (current.Kind != "choice" && current.Kind != "numeric") {
			return nil, fmt.Errorf("Sol 探针字段无效")
		}
		if current.Kind == "numeric" {
			var err error
			current.Tolerance, current.ToleranceMode, err = decodeTolerance(raw.Tolerance)
			if err != nil || current.Question == "" || len(current.Clusters) == 0 {
				return nil, fmt.Errorf("Sol 数值探针无效")
			}
		} else if current.Stem == "" || len(current.Options) != 3 {
			return nil, fmt.Errorf("Sol 选择探针无效")
		}
		if err := validateWeights(current.Weights, 3); err != nil {
			return nil, err
		}
		probes = append(probes, current)
	}
	return probes, nil
}

func validateWeights(weights map[string][]float64, modelCount int) error {
	if len(weights) == 0 {
		return fmt.Errorf("探针缺少评分权重")
	}
	for _, values := range weights {
		if len(values) != modelCount {
			return fmt.Errorf("探针评分权重数量无效")
		}
	}
	return nil
}
