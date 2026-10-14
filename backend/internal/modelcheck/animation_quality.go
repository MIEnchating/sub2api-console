package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

var ErrQualityReview = errors.New("该结果缺少完整的 HTML、提示词或稳定请求信息，不能作为质量标注样本")
var ErrQualityAnnotation = errors.New("质量标注参数无效：请选择有效标签并填写判断理由")

type qualityRepository interface {
	AnimationQualitySample(context.Context, string) (*animationquality.Sample, error)
	SaveAnimationQualitySample(context.Context, animationquality.Sample, int) (animationquality.Sample, error)
	AnimationQualitySamples(context.Context, string, int) (animationquality.SamplePage, error)
}

type AnimationQualityReview struct {
	Prediction   animationquality.Prediction  `json:"prediction"`
	Annotation   *animationquality.Annotation `json:"annotation,omitempty"`
	SampleID     string                       `json:"sample_id"`
	Benchmark    string                       `json:"benchmark"`
	SourceSHA256 string                       `json:"source_sha256"`
	PromptSHA256 string                       `json:"prompt_sha256"`
}

type AnimationQualityAnnotationRequest struct {
	AccountID   string `json:"account_id"`
	RequestID   string `json:"request_id"`
	Label       string `json:"label"`
	Reason      string `json:"reason"`
	SourceGroup string `json:"source_group"`
	Version     int    `json:"version"`
}

func (s *Service) LoadAnimationQualityModel(path string) error {
	model, err := animationquality.Load(path)
	if err != nil {
		return err
	}
	s.qualityModel.Store(model)
	return nil
}

func (s *Service) animationQuality(result AnimationResult) animationquality.Prediction {
	source := result.Source
	if result.SourceTruncated {
		source = ""
	}
	return s.qualityModel.Load().Predict(result.Prompt, source)
}

func (s *Service) qualitySample(ctx context.Context, taskID, accountID, requestID string) (animationquality.Sample, error) {
	var sample animationquality.Sample
	for _, v := range []string{taskID, requestID} {
		if !validQualityText(v, 512, false) {
			return sample, ErrQualityReview
		}
	}
	if len(accountID) > 512 || strings.ContainsFunc(accountID, unicode.IsControl) {
		return sample, ErrQualityReview
	}
	reader, ok := s.tasks.(interface {
		Get(context.Context, string) (taskstore.Task, error)
	})
	if !ok {
		return sample, errors.New("质量结果读取服务尚未就绪")
	}
	task, err := reader.Get(ctx, taskID)
	if err != nil {
		return sample, err
	}
	if task.Status == "running" || task.Status == "queued" || task.Status == "waiting_input" {
		return sample, taskstore.ErrTaskStillActive
	}
	var rows []AnimationResult
	raw, err := json.Marshal(task.Result)
	if err != nil {
		return sample, err
	}
	if task.Skill == animationSkill && (task.Operation == "account-model-animation" || task.Operation == "account-model-combined" || task.Operation == "managed-model-detection") {
		var payload struct {
			Animations []AnimationResult `json:"animations"`
			Mode       string            `json:"mode"`
		}
		if json.Unmarshal(raw, &payload) != nil || payload.Mode == precheckMode {
			return sample, ErrQualityReview
		}
		rows = payload.Animations
	} else if task.Skill == "sub2api-account-onboarding" && task.Operation == "onboarding-probe-probe" {
		var payload struct {
			ProbeResult struct {
				Animation *AnimationResult `json:"animation"`
			} `json:"probe_result"`
		}
		if json.Unmarshal(raw, &payload) != nil || payload.ProbeResult.Animation == nil {
			return sample, ErrQualityReview
		}
		rows = []AnimationResult{*payload.ProbeResult.Animation}
	} else {
		return sample, ErrQualityReview
	}
	var selected *AnimationResult
	for i := range rows {
		if rows[i].AccountID == accountID && rows[i].RequestID == requestID {
			if selected != nil {
				return sample, ErrQualityReview
			}
			selected = &rows[i]
		}
	}
	if selected == nil || selected.Status != "succeeded" || selected.Mode == precheckMode || selected.Precheck != nil || selected.HTML == "" || selected.SourceTruncated || (!utf8.ValidString(selected.Source) || strings.TrimSpace(selected.Source) == "") || len(selected.Source) > animationquality.MaxSourceBytes || !validQualityText(selected.Prompt, maximumAnimationPromptLength, true) {
		return sample, ErrQualityReview
	}
	if _, err = time.Parse(time.RFC3339Nano, selected.CompletedAt); err != nil {
		return sample, ErrQualityReview
	}
	prediction := s.animationQuality(*selected)
	if selected.Quality != nil {
		prediction = *selected.Quality
	}
	sample = animationquality.Sample{ID: animationquality.Hash(taskID + "\x00" + accountID + "\x00" + requestID), TaskID: taskID, AccountID: accountID, RequestID: requestID, Benchmark: animationquality.Benchmark(selected.Prompt), Prompt: selected.Prompt, PromptSHA256: animationquality.Hash(selected.Prompt), HTML: selected.Source, SourceSHA256: animationquality.Hash(selected.Source), Prediction: prediction}
	return sample, nil
}

func validQualityText(v string, max int, multiline bool) bool {
	return utf8.ValidString(v) && strings.TrimSpace(v) != "" && utf8.RuneCountInString(v) <= max && !strings.ContainsFunc(v, func(r rune) bool {
		return unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t'))
	})
}
func qualityReview(sample animationquality.Sample) AnimationQualityReview {
	review := AnimationQualityReview{Prediction: sample.Prediction, SampleID: sample.ID, Benchmark: sample.Benchmark, SourceSHA256: sample.SourceSHA256, PromptSHA256: sample.PromptSHA256}
	if sample.Annotation.Version > 0 {
		review.Annotation = &sample.Annotation
	}
	return review
}
func (s *Service) AnimationQuality(ctx context.Context, taskID, accountID, requestID string) (AnimationQualityReview, error) {
	sample, err := s.qualitySample(ctx, taskID, accountID, requestID)
	if err != nil {
		return AnimationQualityReview{}, err
	}
	repo, ok := s.accounts.(qualityRepository)
	if !ok {
		return AnimationQualityReview{}, errors.New("质量标注存储尚未就绪")
	}
	saved, err := repo.AnimationQualitySample(ctx, sample.ID)
	if err != nil {
		return AnimationQualityReview{}, err
	}
	if saved != nil {
		sample = *saved
	}
	return qualityReview(sample), nil
}
func (s *Service) SaveAnimationQualityAnnotation(ctx context.Context, taskID string, request AnimationQualityAnnotationRequest, actor string) (AnimationQualityReview, error) {
	request.Reason = strings.TrimSpace(request.Reason)
	request.SourceGroup = strings.TrimSpace(request.SourceGroup)
	if (request.Label != "normal" && request.Label != "suspicious" && request.Label != "unknown") || !validQualityText(request.Reason, 2000, true) || !validQualityText(actor, 512, false) || request.Version < 0 || (request.SourceGroup != "" && !validQualityText(request.SourceGroup, 160, false)) {
		return AnimationQualityReview{}, ErrQualityAnnotation
	}
	sample, err := s.qualitySample(ctx, taskID, request.AccountID, request.RequestID)
	if err != nil {
		return AnimationQualityReview{}, err
	}
	if request.SourceGroup == "" {
		request.SourceGroup = "sha256:" + sample.SourceSHA256
	}
	sample.Annotation = animationquality.Annotation{Label: request.Label, Reason: request.Reason, SourceGroup: request.SourceGroup, UpdatedBy: actor}
	repo, ok := s.accounts.(qualityRepository)
	if !ok {
		return AnimationQualityReview{}, errors.New("质量标注存储尚未就绪")
	}
	saved, err := repo.SaveAnimationQualitySample(ctx, sample, request.Version)
	if err != nil {
		return AnimationQualityReview{}, err
	}
	return qualityReview(saved), nil
}
func (s *Service) AnimationQualitySamples(ctx context.Context, after string, limit int) (animationquality.SamplePage, error) {
	if limit < 1 || limit > 100 {
		return animationquality.SamplePage{}, ErrQualityAnnotation
	}
	if after != "" && (len(after) != 64 || strings.Trim(after, "0123456789abcdef") != "") {
		return animationquality.SamplePage{}, ErrQualityAnnotation
	}
	repo, ok := s.accounts.(qualityRepository)
	if !ok {
		return animationquality.SamplePage{}, errors.New("质量标注存储尚未就绪")
	}
	return repo.AnimationQualitySamples(ctx, after, limit)
}
