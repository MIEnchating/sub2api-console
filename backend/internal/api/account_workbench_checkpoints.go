package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchOAuthCheckpoints(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	views, err := service.OAuthCheckpointsScoped(c.Request.Context(), owner, accountworkbench.ExportScope(c.Query("scope")))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, views)
}

func (s *Server) accountWorkbenchSaveOAuthCheckpoint(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "保存检查点参数无效")
		return
	}
	view, err := service.SaveOAuthCheckpoint(c.Request.Context(), owner, c.Param("id"), input.Confirmed)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) accountWorkbenchRestoreOAuthCheckpoint(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.OAuthCheckpointAction
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "恢复检查点参数无效")
		return
	}
	view, err := service.RestoreOAuthCheckpoint(c.Request.Context(), owner, c.Param("id"), input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, view)
}

func (s *Server) accountWorkbenchDeleteOAuthCheckpoint(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.OAuthCheckpointAction
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "删除检查点参数无效")
		return
	}
	if err := service.DeleteOAuthCheckpoint(c.Request.Context(), owner, c.Param("id"), input); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
