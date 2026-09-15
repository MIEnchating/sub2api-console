package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchBatchPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.OAuthBatchPreviewInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "批量授权输入无效")
		return
	}
	result, err := service.PreviewOAuthBatch(c.Request.Context(), owner, input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDiscardBatchPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	service.DeleteOAuthBatchPreview(owner, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) accountWorkbenchStartBatch(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"preview_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "批量授权确认参数无效")
		return
	}
	result, err := service.StartOAuthBatch(c.Request.Context(), owner, input.PreviewID, input.Confirmed)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (s *Server) accountWorkbenchReadBatch(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.ReadOAuthBatch(owner, c.Param("id"))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchCancelBatch(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	if err := service.CancelOAuthBatch(owner, c.Param("id")); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}

func (s *Server) accountWorkbenchBatchImportPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.OAuthPreviewInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "批量授权导入预览参数无效")
		return
	}
	result, err := service.PreviewOAuthBatchImport(c.Request.Context(), owner, c.Param("id"), input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
