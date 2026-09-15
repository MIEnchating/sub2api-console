package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchLocalExports(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.LocalExports(c.Request.Context(), owner)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDeleteLocalExport(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	if err := service.DeleteLocalExport(c.Request.Context(), owner, c.Param("id")); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
