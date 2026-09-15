package api

import (
	"context"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
)

type animationCheckService interface {
	EnqueueAnimation(context.Context, modelcheck.AnimationRequest) (taskstore.Task, error)
	AnimationHistory(context.Context) ([]taskstore.Task, error)
	AnimationSchedules() []modelcheck.AnimationScheduleView
	SaveAnimationSchedule(context.Context, modelcheck.AnimationSchedule, string) ([]modelcheck.AnimationScheduleView, error)
}

func (s *Server) customAnimationModels(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	service, ok := s.modelChecks.(interface {
		CustomAnimationModels(context.Context, modelcheck.AnimationCustomEndpoint) ([]string, error)
	})
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "模型列表服务尚未就绪")
		return
	}
	var payload modelcheck.AnimationCustomEndpoint
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "模型列表参数无效")
		return
	}
	models, err := service.CustomAnimationModels(c.Request.Context(), payload)
	if err != nil {
		writeError(c, http.StatusBadGateway, "模型列表读取失败："+err.Error())
		return
	}
	c.JSON(http.StatusOK, struct {
		Models []string `json:"models"`
	}{Models: models})
}

func (s *Server) animationService(c *gin.Context) (animationCheckService, bool) {
	service, ok := s.modelChecks.(animationCheckService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "动画检测服务尚未就绪")
	}
	c.Header("Cache-Control", "no-store")
	return service, ok
}

func (s *Server) runAnimationCheck(c *gin.Context) {
	service, ok := s.animationService(c)
	if !ok {
		return
	}
	var payload modelcheck.AnimationRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "动画检测参数无效")
		return
	}
	task, err := service.EnqueueAnimation(c.Request.Context(), payload)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) animationCheckHistory(c *gin.Context) {
	service, ok := s.animationService(c)
	if !ok {
		return
	}
	tasks, err := service.AnimationHistory(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "动画检测历史读取失败，请重试")
		return
	}
	c.JSON(http.StatusOK, tasks)
}

func (s *Server) animationCheckSchedules(c *gin.Context) {
	service, ok := s.animationService(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, service.AnimationSchedules())
}

func (s *Server) saveAnimationCheckSchedule(c *gin.Context) {
	service, ok := s.animationService(c)
	if !ok {
		return
	}
	var payload modelcheck.AnimationSchedule
	if err := bindRequestJSON(c, &payload); err != nil || payload.AccountID != c.Param("id") {
		writeError(c, http.StatusUnprocessableEntity, "自动检测账号 ID 或参数无效")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	result, err := service.SaveAnimationSchedule(c.Request.Context(), payload, actor)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
