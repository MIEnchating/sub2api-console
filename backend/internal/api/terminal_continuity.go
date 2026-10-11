package api

import (
	"context"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
)

type terminalContinuityService interface {
	TerminalContinuityHistory(context.Context) ([]taskstore.Task, error)
}

func (s *Server) runTerminalContinuity(c *gin.Context) {
	writeError(c, http.StatusGone, "终端检测已移除，请使用动画检测或前置检测")
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
