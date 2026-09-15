package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchProfileExportPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.ProfileExportInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "登录资料导出范围无效")
		return
	}
	view, err := service.PreviewProfileExport(c.Request.Context(), owner, input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) accountWorkbenchExportProfiles(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"preview_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "登录资料导出确认参数无效")
		return
	}
	task, err := service.ExportProfiles(c.Request.Context(), owner, input.PreviewID, input.Confirmed)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, task)
}
