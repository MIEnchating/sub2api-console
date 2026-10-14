package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
)

type animationQualityService interface {
	AnimationQuality(context.Context, string, string, string) (modelcheck.AnimationQualityReview, error)
	SaveAnimationQualityAnnotation(context.Context, string, modelcheck.AnimationQualityAnnotationRequest, string) (modelcheck.AnimationQualityReview, error)
	AnimationQualitySamples(context.Context, string, int) (animationquality.SamplePage, error)
}

func (s *Server) animationQualityService(c *gin.Context) animationQualityService {
	c.Header("Cache-Control", "no-store")
	service, ok := s.modelChecks.(animationQualityService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "质量标注服务尚未就绪")
		return nil
	}
	return service
}
func qualityError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, taskstore.ErrNotFound):
		writeError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, taskstore.ErrTaskStillActive), errors.Is(err, business.ErrQualityAnnotationConflict):
		writeError(c, http.StatusConflict, err.Error())
	case errors.Is(err, modelcheck.ErrQualityReview), errors.Is(err, modelcheck.ErrQualityAnnotation):
		writeError(c, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(c, http.StatusInternalServerError, "质量标注操作失败，请重试")
	}
}
func (s *Server) getAnimationQuality(c *gin.Context) {
	service := s.animationQualityService(c)
	if service == nil {
		return
	}
	review, err := service.AnimationQuality(c.Request.Context(), c.Param("task_id"), c.Query("account_id"), c.Query("request_id"))
	if err != nil {
		qualityError(c, err)
		return
	}
	c.JSON(http.StatusOK, review)
}
func (s *Server) saveAnimationQuality(c *gin.Context) {
	service := s.animationQualityService(c)
	if service == nil {
		return
	}
	var payload modelcheck.AnimationQualityAnnotationRequest
	if bindRequestJSON(c, &payload) != nil {
		writeError(c, http.StatusUnprocessableEntity, modelcheck.ErrQualityAnnotation.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	review, err := service.SaveAnimationQualityAnnotation(c.Request.Context(), c.Param("task_id"), payload, actor)
	if err != nil {
		qualityError(c, err)
		return
	}
	c.JSON(http.StatusOK, review)
}
func (s *Server) listAnimationQualitySamples(c *gin.Context) {
	service := s.animationQualityService(c)
	if service == nil {
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		writeError(c, http.StatusUnprocessableEntity, "每批样本数量必须为 1～100")
		return
	}
	page, err := service.AnimationQualitySamples(c.Request.Context(), c.Query("after"), limit)
	if err != nil {
		qualityError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}
