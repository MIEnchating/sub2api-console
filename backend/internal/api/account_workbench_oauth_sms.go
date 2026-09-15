package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchReadOAuthSMS(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.ReadOAuthSMSAttachment(c.Request.Context(), owner, c.Param("id"), accountworkbench.ExportScope(c.Query("scope")))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchAttachOAuthSMS(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.OAuthSMSAttachmentInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, 422, "接码配置参数无效")
		return
	}
	if err := service.AttachOAuthSMS(c.Request.Context(), owner, c.Param("id"), input); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"accepted": true})
}
