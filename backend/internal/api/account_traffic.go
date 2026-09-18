package api

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/gin-gonic/gin"
	"net/http"
)

type accountTrafficReader interface {
	AccountTraffic(context.Context) (adminclient.AccountTrafficSnapshot, error)
}

func (s *Server) accountTraffic(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	reader, ok := s.traceReader.(accountTrafficReader)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "实时流量服务尚未就绪，请稍后重试")
		return
	}
	snapshot, err := reader.AccountTraffic(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusBadGateway, "实时流量读取失败，请检查 Sub2API 管理连接和运维实时监控后重试")
		return
	}
	c.JSON(http.StatusOK, snapshot)
}
