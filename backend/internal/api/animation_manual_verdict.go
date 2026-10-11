package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
)

func (s *Server) setAnimationManualVerdict(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	service, ok := s.modelChecks.(interface {
		SetAnimationManualVerdict(context.Context, string, modelcheck.AnimationManualVerdictRequest, string) (taskstore.Task, error)
	})
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "手动标记服务尚未就绪")
		return
	}
	var payload modelcheck.AnimationManualVerdictRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "手动标记参数无效")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := service.SetAnimationManualVerdict(c.Request.Context(), c.Param("task_id"), payload, actor)
	if err != nil {
		switch {
		case errors.Is(err, taskstore.ErrNotFound):
			writeError(c, http.StatusNotFound, err.Error())
		case errors.Is(err, taskstore.ErrTaskStillActive):
			writeError(c, http.StatusConflict, err.Error())
		case errors.Is(err, modelcheck.ErrInvalidAnimationVerdict):
			writeError(c, http.StatusUnprocessableEntity, err.Error())
		default:
			writeError(c, http.StatusInternalServerError, "手动标记保存失败，请重试")
		}
		return
	}
	c.JSON(http.StatusOK, task)
}
