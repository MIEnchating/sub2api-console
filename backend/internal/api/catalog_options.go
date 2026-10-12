package api

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/gin-gonic/gin"
	"net/http"
)

type catalogOptionsStore interface {
	GroupMembershipOptions(context.Context) ([]business.GroupMembershipOption, error)
	GroupOptions(context.Context) ([]business.GroupOption, error)
	GroupConfigurations(context.Context) ([]business.GroupConfiguration, error)
	AccountOptions(context.Context) ([]business.AccountOption, error)
	UpstreamOptions(context.Context) ([]business.UpstreamOption, error)
}

func (s *Server) catalogOptions(c *gin.Context) {
	store, ok := s.business.(catalogOptionsStore)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "目录读取服务尚未就绪，请稍后重试")
		return
	}
	var result any
	var err error
	switch c.Param("kind") {
	case "group-memberships":
		result, err = store.GroupMembershipOptions(c.Request.Context())
	case "group-configurations":
		result, err = store.GroupConfigurations(c.Request.Context())
	case "groups":
		result, err = store.GroupOptions(c.Request.Context())
	case "accounts":
		result, err = store.AccountOptions(c.Request.Context())
	case "upstreams":
		result, err = store.UpstreamOptions(c.Request.Context())
	default:
		writeError(c, http.StatusNotFound, "目录不存在")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "目录读取失败，请重试")
		return
	}
	c.JSON(http.StatusOK, result)
}
