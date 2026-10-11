package api

import (
	"context"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/gin-gonic/gin"
)

func (s *Server) addModelSyncBlock(c *gin.Context) {
	service, ok := s.accountModelSync.(interface {
		AddModelSyncBlock(context.Context, string, string) (business.AccountModelSyncSettings, error)
	})
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "账号模型同步服务尚未就绪")
		return
	}
	var payload struct {
		Model string `json:"model"`
	}
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "全局屏蔽模型参数无效")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	settings, err := service.AddModelSyncBlock(c.Request.Context(), payload.Model, actor)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, settings)
}
