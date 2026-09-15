package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchCheckpointSecurity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.SecurityCheckpointStartInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "检查点安全操作参数无效")
		return
	}
	result, err := service.StartCheckpointSecurity(c.Request.Context(), owner, c.Param("id"), input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (s *Server) accountWorkbenchConfirmSecurityIdentity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.SecurityIdentityInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "账号身份确认参数无效")
		return
	}
	if err := service.ConfirmSecurityIdentity(c.Request.Context(), owner, c.Param("id"), input); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"accepted": true})
}

func (s *Server) accountWorkbenchOAuthAfterSecurity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "安全操作后授权参数无效")
		return
	}
	result, err := service.OAuthAfterSecurity(c.Request.Context(), owner, c.Param("id"), input.Confirmed)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}
