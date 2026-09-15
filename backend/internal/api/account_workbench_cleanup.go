package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchCleanupPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.CleanupPreviewInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "账号清理范围无效")
		return
	}
	view, err := service.PreviewAccountCleanup(c.Request.Context(), owner, input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) accountWorkbenchCleanup(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"preview_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "账号清理确认参数无效")
		return
	}
	task, err := service.CleanupAccounts(c.Request.Context(), owner, input.PreviewID, input.Confirmed)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, task)
}

func (s *Server) accountWorkbenchDiscardCleanupPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	service.DeleteCleanupPreview(owner, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
