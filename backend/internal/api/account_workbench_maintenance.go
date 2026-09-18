package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchMaintenance(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	ctx, owner := c.Request.Context(), workbenchOwner(c)
	var result any
	var err error
	status := http.StatusOK
	if c.Request.Method == http.MethodGet {
		result, err = s.accountWorkbench.Maintenance(ctx, owner)
	} else {
		switch c.Param("action") {
		case "preview":
			var input accountworkbench.MaintenanceSettings
			if bindRequestJSON(c, &input) != nil {
				writeError(c, http.StatusUnprocessableEntity, "维护设置格式无效")
				return
			}
			result, err = s.accountWorkbench.PreviewMaintenance(ctx, owner, input)
		case "configure":
			var input accountworkbench.RunConfirmation
			if bindRequestJSON(c, &input) != nil {
				writeError(c, http.StatusUnprocessableEntity, "维护预览版本无效")
				return
			}
			result, err = s.accountWorkbench.ConfigureMaintenance(ctx, owner, input)
		case "check", "stop":
			var input struct {
				Revision int64 `json:"revision"`
			}
			if bindRequestJSON(c, &input) != nil {
				writeError(c, http.StatusUnprocessableEntity, "维护配置版本无效")
				return
			}
			if c.Param("action") == "check" {
				result, err = s.accountWorkbench.CheckMaintenance(ctx, owner, input.Revision)
				status = http.StatusAccepted
			} else {
				result, err = s.accountWorkbench.StopMaintenance(ctx, owner, input.Revision)
			}
		default:
			writeError(c, http.StatusNotFound, "不支持此维护操作")
			return
		}
	}
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(status, result)
}
