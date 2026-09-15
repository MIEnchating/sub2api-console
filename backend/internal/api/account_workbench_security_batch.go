package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchSecurityBatchPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.SecurityBatchPreviewInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "批量安全操作范围无效")
		return
	}
	result, err := service.PreviewSecurityBatch(c.Request.Context(), owner, input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDiscardSecurityBatchPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	service.DeleteSecurityBatchPreview(owner, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) accountWorkbenchStartSecurityBatch(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"preview_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "批量安全操作确认参数无效")
		return
	}
	result, err := service.StartSecurityBatch(c.Request.Context(), owner, input.PreviewID, input.Confirmed)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (s *Server) accountWorkbenchReadSecurityBatch(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.ReadSecurityBatch(owner, c.Param("id"))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchCancelSecurityBatch(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	if err := service.CancelSecurityBatch(owner, c.Param("id")); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}
