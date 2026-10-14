package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/gin-gonic/gin"
)

type qualitySettingsService interface {
	AnimationQualitySettings() modelcheck.AnimationQualitySettings
	SaveAnimationQualitySettings(context.Context, modelcheck.AnimationQualitySettings, string) (modelcheck.AnimationQualitySettings, error)
}

func (s *Server) animationQualitySettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	service, ok := s.modelChecks.(qualitySettingsService)
	if !ok {
		writeError(c, 503, "自动质量标注设置尚未就绪")
		return
	}
	c.JSON(http.StatusOK, service.AnimationQualitySettings())
}
func (s *Server) saveAnimationQualitySettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	service, ok := s.modelChecks.(qualitySettingsService)
	if !ok {
		writeError(c, 503, "自动质量标注设置尚未就绪")
		return
	}
	var payload modelcheck.AnimationQualitySettings
	if bindRequestJSON(c, &payload) != nil {
		writeError(c, 422, modelcheck.ErrQualitySettings.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, 500, "控制台会话读取失败")
		return
	}
	value, err := service.SaveAnimationQualitySettings(c.Request.Context(), payload, actor)
	if err != nil {
		switch {
		case errors.Is(err, modelcheck.ErrQualitySettingsConflict):
			writeError(c, 409, err.Error())
		case errors.Is(err, modelcheck.ErrQualitySettings):
			writeError(c, 422, err.Error())
		default:
			writeError(c, 500, "自动质量标注设置保存失败，请重试")
		}
		return
	}
	c.JSON(http.StatusOK, value)
}
