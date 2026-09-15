package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchStartOAuth(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.OAuthStartInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, 422, "授权参数无效")
		return
	}
	result, err := service.StartOAuthWithInput(c.Request.Context(), owner, input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (s *Server) accountWorkbenchSMSOptions(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.OAuthSMSInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, 422, "短信查询参数无效")
		return
	}
	result, err := service.SMSOptions(c.Request.Context(), input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchReadOAuth(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.ReadOAuth(c.Request.Context(), owner, c.Param("id"))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchInputOAuth(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input browserlogin.Input
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, 422, "授权页面操作参数无效")
		return
	}
	if err := input.Validate(); err != nil {
		writeError(c, 422, err.Error())
		return
	}
	if err := service.InputOAuth(c.Request.Context(), owner, c.Param("id"), input); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"accepted": true})
}

func (s *Server) accountWorkbenchFinishOAuth(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	if err := service.FinishOAuth(owner, c.Param("id")); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"accepted": true})
}

func (s *Server) accountWorkbenchCancelOAuth(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	if err := service.CancelOAuth(owner, c.Param("id")); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}

func (s *Server) accountWorkbenchOAuthPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.OAuthPreviewInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, 422, "授权导入预览参数无效")
		return
	}
	result, err := service.PreviewOAuth(c.Request.Context(), owner, c.Param("id"), input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
