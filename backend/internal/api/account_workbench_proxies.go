package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) accountWorkbenchProxies(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	result, err := s.accountWorkbench.PlatformProxies(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
