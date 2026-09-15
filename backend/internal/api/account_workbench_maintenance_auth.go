package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) accountWorkbenchMaintenanceAuthorization(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.MaintenanceAuthorization(c.Request.Context(), owner)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
func (s *Server) accountWorkbenchAttachMaintenance(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		Revision  int64 `json:"revision"`
		Confirmed bool  `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil || !input.Confirmed || input.Revision < 1 {
		writeError(c, http.StatusUnprocessableEntity, "请先确认本次登录会话的自动重新授权范围")
		return
	}
	if err := service.BindMaintenanceOwner(c.Request.Context(), owner, input.Revision); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	result, err := service.MaintenanceAuthorization(c.Request.Context(), owner)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
