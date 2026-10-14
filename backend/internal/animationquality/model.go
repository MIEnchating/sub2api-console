package animationquality

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"unicode/utf8"
)

type Model struct {
	SchemaVersion          int            `json:"schema_version"`
	ModelVersion           string         `json:"model_version"`
	Benchmark              string         `json:"benchmark"`
	PromptSHA256           string         `json:"prompt_sha256"`
	PreprocessVersion      string         `json:"preprocess_version"`
	Lowercase              bool           `json:"lowercase"`
	SublinearTF            bool           `json:"sublinear_tf"`
	SmoothIDF              bool           `json:"smooth_idf"`
	Analyzer               string         `json:"analyzer"`
	NgramMin               int            `json:"ngram_min"`
	NgramMax               int            `json:"ngram_max"`
	Norm                   string         `json:"norm"`
	Vocabulary             map[string]int `json:"vocabulary"`
	IDF                    []float64      `json:"idf"`
	Weights                []float64      `json:"weights"`
	Intercept              float64        `json:"intercept"`
	PositiveClass          string         `json:"positive_class"`
	Threshold              float64        `json:"threshold"`
	UnknownMargin          float64        `json:"unknown_margin"`
	TrainingDataVersion    string         `json:"training_data_version"`
	TrainingLibraryVersion string         `json:"training_library_version"`
	ValidationReport       string         `json:"validation_report"`
	Accepted               bool           `json:"accepted"`
}

type Detector struct {
	model Model
	hash  string
}

// Load is called at startup only. Absence is intentional; a configured invalid
// model is an error, never a silent fallback to fabricated weights.
func Load(path string) (*Detector, error) {
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("质量模型路径必须为绝对路径")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (32<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 32<<20 {
		return nil, errors.New("质量模型超过 32 MiB")
	}
	return Decode(raw)
}

func Decode(raw []byte) (*Detector, error) {
	var m Model
	var fields map[string]json.RawMessage
	if len(raw) > 32<<20 || json.Unmarshal(raw, &fields) != nil {
		return nil, errors.New("质量模型 JSON 无效或超过长度限制")
	}
	for _, key := range []string{"schema_version", "model_version", "benchmark", "prompt_sha256", "preprocess_version", "lowercase", "sublinear_tf", "smooth_idf", "analyzer", "ngram_min", "ngram_max", "norm", "vocabulary", "idf", "weights", "intercept", "positive_class", "threshold", "unknown_margin", "training_data_version", "training_library_version", "validation_report", "accepted"} {
		if value, ok := fields[key]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errors.New("质量模型缺少必要契约字段：" + key)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if !utf8.Valid(raw) || decoder.Decode(&m) != nil {
		return nil, errors.New("质量模型 JSON 无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("质量模型包含多余内容")
	}
	if m.SchemaVersion != 1 || m.ModelVersion == "" || m.Benchmark != "pelican-v1" || m.PromptSHA256 != Hash(PelicanPrompt) || m.PreprocessVersion != "sklearn-char-v1" || m.Lowercase || m.SublinearTF || !m.SmoothIDF || m.Analyzer != "char" || m.NgramMin != 3 || m.NgramMax != 5 || m.Norm != "l2" || m.PositiveClass != "suspicious" {
		return nil, errors.New("质量模型预处理或题目契约不匹配")
	}
	if !m.Accepted || m.TrainingDataVersion == "" || m.TrainingLibraryVersion == "" || m.ValidationReport == "" {
		return nil, errors.New("质量模型缺少验收或训练信息")
	}
	n := len(m.Vocabulary)
	if n == 0 || n > 500000 || len(m.IDF) != n || len(m.Weights) != n {
		return nil, errors.New("质量模型词表与权重长度不一致")
	}
	seen := make([]bool, n)
	for term, index := range m.Vocabulary {
		length := utf8.RuneCountInString(term)
		if index < 0 || index >= n || seen[index] || length < 3 || length > 5 {
			return nil, errors.New("质量模型词表索引或字符长度无效")
		}
		seen[index] = true
	}
	valid := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= 1e6 }
	for i := range n {
		if !valid(m.IDF[i]) || m.IDF[i] <= 0 || !valid(m.Weights[i]) {
			return nil, errors.New("质量模型数值无效")
		}
	}
	if !valid(m.Intercept) || !valid(m.Threshold) || !valid(m.UnknownMargin) || m.Threshold <= 0 || m.Threshold >= 1 || m.UnknownMargin < 0 || m.Threshold-m.UnknownMargin < 0 || m.Threshold+m.UnknownMargin > 1 {
		return nil, errors.New("质量模型阈值无效")
	}
	return &Detector{model: m, hash: Hash(string(raw))}, nil
}
