package api

import (
	"context"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/gin-gonic/gin"
)

type upstreamListStore interface {
	UpstreamList(context.Context) (business.UpstreamSummary, error)
}
type upstreamUsageStore interface {
	UpstreamUsage(context.Context) ([]business.UpstreamUsage, error)
}

func (s *Server) upstreamUsage(c *gin.Context) {
	store, ok := s.business.(upstreamUsageStore)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "上游用量服务尚未就绪，请稍后重试")
		return
	}
	result, err := store.UpstreamUsage(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "上游用量读取失败，请重试")
		return
	}
	c.JSON(http.StatusOK, result)
}
