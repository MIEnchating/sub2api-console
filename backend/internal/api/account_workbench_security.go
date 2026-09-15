package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchSecuritySource(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.SecuritySource(c.Request.Context(), owner, c.Param("id"))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchStartSecurity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.SecurityStartInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "账号安全操作参数无效")
		return
	}
	result, err := service.StartSecurity(c.Request.Context(), owner, input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (s *Server) accountWorkbenchReadSecurity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.ReadSecurity(c.Request.Context(), owner, c.Param("id"))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchInputSecurity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input browserlogin.Input
	if err := bindRequestJSON(c, &input); err != nil || input.Validate() != nil {
		writeError(c, http.StatusUnprocessableEntity, "安全验证页面操作参数无效")
		return
	}
	if err := service.InputSecurity(c.Request.Context(), owner, c.Param("id"), input); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"accepted": true})
}

func (s *Server) accountWorkbenchContinueSecurity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	if err := service.ContinueSecurity(c.Request.Context(), owner, c.Param("id")); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"accepted": true})
}

func (s *Server) accountWorkbenchCancelSecurity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	if err := service.CancelSecurity(owner, c.Param("id")); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}
