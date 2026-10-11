package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) newAPIRemoteGroups(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "New API 管理服务尚未就绪")
		return
	}
	result, err := s.newAPIManagement.Groups(c.Request.Context(), c.Param("platform_id"))
	if err != nil {
		writeNewAPIError(c, err, http.StatusBadGateway)
		return
	}
	c.JSON(http.StatusOK, result)
}
