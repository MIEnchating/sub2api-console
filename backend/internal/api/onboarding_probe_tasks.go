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
	if action != "models" && action != "probe" && action != "cleanup" {
		writeError(c, http.StatusUnprocessableEntity, "不支持的探活操作")
		return
	}
	host, groupID, model, mode, ok := onboardingProbePayload(c, action == "probe")
	if !ok {
		return
	}
	task, err := service.EnqueueProbe(c.Request.Context(), action, host, groupID, model, mode)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}
