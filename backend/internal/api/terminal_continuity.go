package api

import (
	"context"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
)

type terminalContinuityService interface {
	EnqueueTerminalContinuity(context.Context, modelcheck.TerminalContinuityRequest) (taskstore.Task, error)
	TerminalContinuityHistory(context.Context) ([]taskstore.Task, error)
}

func (s *Server) runTerminalContinuity(c *gin.Context) {
	service, ok := s.modelChecks.(terminalContinuityService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "终端续接检测尚未就绪")
		return
	}
	var input modelcheck.TerminalContinuityRequest
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "终端续接检测参数无效")
		return
	}
	task, err := service.EnqueueTerminalContinuity(c.Request.Context(), input)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) terminalContinuityHistory(c *gin.Context) {
	service, ok := s.modelChecks.(terminalContinuityService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "终端续接检测尚未就绪")
		return
	}
	tasks, err := service.TerminalContinuityHistory(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "终端续接检测记录读取失败，请重试")
		return
	}
	c.JSON(http.StatusOK, tasks)
}
