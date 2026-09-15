package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchExportInput(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"preview_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "私有转换确认参数无效")
		return
	}
	result, err := service.ExportInput(c.Request.Context(), owner, input.PreviewID, input.Confirmed)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (s *Server) accountWorkbenchExportPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.ExportPreviewInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "导出账号参数无效")
		return
	}
	result, err := service.PreviewExport(c.Request.Context(), owner, input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDiscardExportPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	service.DeleteExportPreview(owner, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) accountWorkbenchExport(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"preview_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "导出确认参数无效")
		return
	}
	task, err := service.Export(c.Request.Context(), owner, input.PreviewID, input.Confirmed)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, task)
}

func (s *Server) accountWorkbenchExports(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.Exports(c.Request.Context(), owner)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDeleteExport(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	if err := service.DeleteExport(c.Request.Context(), owner, c.Param("id")); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) accountWorkbenchRetryPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.RetryPreviewInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "重试预览参数无效")
		return
	}
	result, err := service.RetryPreview(c.Request.Context(), owner, input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
