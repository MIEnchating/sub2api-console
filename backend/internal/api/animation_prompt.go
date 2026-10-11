package api

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/gin-gonic/gin"
	"net/http"
)

type animationPromptService interface {
	AnimationPromptSettings() modelcheck.AnimationPromptSettings
	SaveAnimationPrompt(context.Context, modelcheck.AnimationPromptSettings, string) (modelcheck.AnimationPromptSettings, error)
}

func (s *Server) animationPromptSettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	service, ok := s.modelChecks.(animationPromptService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "动画提示词服务尚未就绪")
		return
	}
	c.JSON(http.StatusOK, service.AnimationPromptSettings())
}
func (s *Server) saveAnimationPrompt(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	service, ok := s.modelChecks.(animationPromptService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "动画提示词服务尚未就绪")
		return
	}
	var payload modelcheck.AnimationPromptSettings
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "动画提示词参数无效")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	value, err := service.SaveAnimationPrompt(c.Request.Context(), payload, actor)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, value)
}
