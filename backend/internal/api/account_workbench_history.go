package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchQueryHistory(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input taskstore.HistoryQuery
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "历史查询参数无效")
		return
	}
	result, err := service.QueryHistory(c.Request.Context(), input)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchActiveHistory(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.ActiveHistory(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDeleteHistory(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.HistoryActionInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "处理记录确认参数无效")
		return
	}
	if err := service.DeleteHistory(c.Request.Context(), input); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": len(input.Items)})
}

func (s *Server) accountWorkbenchCancelHistory(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.HistoryActionInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "任务取消确认参数无效")
		return
	}
	result, err := service.CancelHistory(c.Request.Context(), input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"items": result})
}
