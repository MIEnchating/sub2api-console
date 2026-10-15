package modelcheck

import (
	"context"
	"errors"
	"strings"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

var ErrQualityConfirmation = errors.New("当前自动结果无法收录：请等待评审完成，并确认有明确标签和理由后重试")

type AnimationQualityConfirmationRequest struct {
	AccountID        string `json:"account_id"`
	RequestID        string `json:"request_id"`
	Version          int    `json:"version"`
	RecheckVersion   int    `json:"recheck_version"`
	PredictionSHA256 string `json:"prediction_sha256"`
}

func (s *Service) ConfirmAnimationQuality(ctx context.Context, taskID string, request AnimationQualityConfirmationRequest, actor string) (AnimationQualityReview, error) {
	if request.Version < 0 || request.RecheckVersion < 0 || len(request.PredictionSHA256) != 64 || strings.Trim(request.PredictionSHA256, "0123456789abcdef") != "" || !validQualityText(actor, 512, false) {
		return AnimationQualityReview{}, ErrQualityConfirmation
	}
	sample, err := s.qualitySample(ctx, taskID, request.AccountID, request.RequestID)
	if err != nil {
		return AnimationQualityReview{}, err
	}
	latest, err := s.withQualityRecheck(ctx, sample)
	if err != nil {
		return AnimationQualityReview{}, err
	}
	if latest.RecheckVersion != request.RecheckVersion || latest.PredictionSHA256 != request.PredictionSHA256 {
		return AnimationQualityReview{}, business.ErrQualityConfirmationConflict
	}
	prediction := latest.Prediction
	if latest.Recheck != nil && (latest.Recheck.Status == "queued" || latest.Recheck.Status == "running") || prediction.Source != "model_review" || (prediction.Status != "normal" && prediction.Status != "suspicious") || prediction.Reason != "" || !validQualityText(prediction.Explanation, 2000, true) {
		return AnimationQualityReview{}, ErrQualityConfirmation
	}
	repo, ok := s.accounts.(qualityRepository)
	if !ok {
		return AnimationQualityReview{}, errors.New("质量标注存储尚未就绪")
	}
	previous, err := repo.AnimationQualitySample(ctx, sample.ID)
	if err != nil {
		return AnimationQualityReview{}, err
	}
	group := "sha256:" + sample.SourceSHA256
	if previous != nil {
		group = previous.Annotation.SourceGroup
	}
	sample.Prediction = prediction
	sample.Annotation = q.Annotation{
		Source: "confirmed_auto", Label: prediction.Status, Reason: prediction.Explanation,
		SourceGroup: group, UpdatedBy: actor, PredictionSHA256: request.PredictionSHA256,
		ConfirmedPrediction: &prediction, RecheckVersion: request.RecheckVersion,
	}
	saved, err := repo.SaveAnimationQualitySample(ctx, sample, request.Version)
	if err != nil {
		return AnimationQualityReview{}, err
	}
	return s.withQualityRecheck(ctx, saved)
}
