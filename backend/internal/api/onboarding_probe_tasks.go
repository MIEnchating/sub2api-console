package api

import (
	"context"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
)

type onboardingProbeEnqueuer interface {
	EnqueueProbe(context.Context, string, string, string, string, string) (taskstore.Task, error)
}

func (s *Server) onboardingProbeTask(c *gin.Context) {
	service, ok := s.onboarding.(onboardingProbeEnqueuer)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "接入前探活任务服务尚未就绪")
		return
	}
	action := c.Param("action")
	if action != "models" && action != "model-options" && action != "probe" && action != "cleanup" {
		writeError(c, http.StatusUnprocessableEntity, "不支持的探活操作")
		return
	}
	host, groupID, model, mode, promptID, ok := onboardingProbePayload(c, action == "probe")
	if !ok {
		return
	}
	var task taskstore.Task
	var err error
	if promptID != "" {
		selected, available := s.onboarding.(interface {
			EnqueueProbeWithPrompt(context.Context, string, string, string, string, string, string) (taskstore.Task, error)
		})
		if !available {
			writeError(c, http.StatusServiceUnavailable, "动画提示词选择服务尚未就绪")
			return
		}
		task, err = selected.EnqueueProbeWithPrompt(c.Request.Context(), action, host, groupID, model, mode, promptID)
	} else {
		task, err = service.EnqueueProbe(c.Request.Context(), action, host, groupID, model, mode)
	}
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}
