package api

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) accountWorkbenchImportModel(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	if c.Request.Method == http.MethodGet {
		result, err := s.accountWorkbench.ImportModel(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusConflict, err.Error())
			return
		}
		c.JSON(http.StatusOK, result)
		return
	}
	var input accountworkbench.ImportModelSettings
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "检测模型设置无效")
		return
	}
	result, err := s.accountWorkbench.SaveImportModel(c.Request.Context(), input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
