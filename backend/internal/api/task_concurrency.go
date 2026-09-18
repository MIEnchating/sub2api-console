package api

import (
	"errors"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/tasksettings"
	"github.com/gin-gonic/gin"
)

func (s *Server) taskConcurrency(c *gin.Context) {
	if s.taskSettings == nil {
		writeError(c, http.StatusServiceUnavailable, "任务并发设置暂不可用")
		return
	}
	c.JSON(http.StatusOK, s.taskSettings.Snapshot())
}
func (s *Server) updateTaskConcurrency(c *gin.Context) {
	if s.taskSettings == nil {
		writeError(c, http.StatusServiceUnavailable, "任务并发设置暂不可用")
		return
	}
	var input tasksettings.Input
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "任务并发参数无效")
		return
	}
	result, err := s.taskSettings.Update(c.Request.Context(), input)
	if errors.Is(err, tasksettings.ErrConflict) {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, tasksettings.ErrInvalid) {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "任务并发保存失败，请重试")
		return
	}
	c.JSON(http.StatusOK, result)
}
