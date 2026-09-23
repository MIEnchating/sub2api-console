package api

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
	"net/http"
)

type detectionTaskService interface {
	DetectionTasks() []modelcheck.DetectionTaskView
	SaveDetectionTask(context.Context, modelcheck.DetectionTask, string) ([]modelcheck.DetectionTaskView, error)
	DeleteDetectionTask(context.Context, string, int, string) ([]modelcheck.DetectionTaskView, error)
	RunDetectionTask(context.Context, string, int) (taskstore.Task, error)
}

func (s *Server) detectionTaskEndpoint(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	service, ok := s.modelChecks.(detectionTaskService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "检测任务管理尚未就绪")
		return
	}
	if c.Request.Method == http.MethodGet {
		c.JSON(http.StatusOK, service.DetectionTasks())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	if c.Request.Method == http.MethodPut {
		var value modelcheck.DetectionTask
		if err := bindRequestJSON(c, &value); err != nil {
			writeError(c, http.StatusUnprocessableEntity, "检测任务参数无效")
			return
		}
		values, err := service.SaveDetectionTask(c.Request.Context(), value, actor)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err.Error())
			return
		}
		c.JSON(http.StatusOK, values)
		return
	}
	var input struct {
		Version int `json:"version"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "检测任务版本无效")
		return
	}
	if c.Request.Method == http.MethodDelete {
		values, err := service.DeleteDetectionTask(c.Request.Context(), c.Param("id"), input.Version, actor)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err.Error())
			return
		}
		c.JSON(http.StatusOK, values)
		return
	}
	task, err := service.RunDetectionTask(c.Request.Context(), c.Param("id"), input.Version)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}
