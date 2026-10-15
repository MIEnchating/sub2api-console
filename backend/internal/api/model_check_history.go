package api

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type modelCheckHistoryItem struct {
	ID        string `json:"id"`
	Skill     string `json:"skill"`
	Operation string `json:"operation"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type modelCheckHistoryResponse struct {
	Items   []modelCheckHistoryItem `json:"items"`
	HasMore bool                    `json:"has_more"`
}

func (s *Server) modelCheckHistory(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 100000 {
		writeError(c, http.StatusUnprocessableEntity, "page 必须在 1 到 100000 之间")
		return
	}
	if s.tasks == nil {
		writeError(c, http.StatusServiceUnavailable, "任务服务尚未就绪")
		return
	}
	repository, ok := s.tasks.(interface {
		ModelCheckHistoryPage(context.Context, int) ([]taskstore.Task, bool, error)
	})
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "模型检测历史服务尚未就绪")
		return
	}
	rows, more, err := repository.ModelCheckHistoryPage(c.Request.Context(), page)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "模型检测历史读取失败，请重试")
		return
	}
	response := modelCheckHistoryResponse{Items: make([]modelCheckHistoryItem, 0, len(rows)), HasMore: more}
	for _, task := range rows {
		response.Items = append(response.Items, modelCheckHistoryItem{ID: task.ID, Skill: task.Skill, Operation: task.Operation, Status: task.Status, Progress: task.Progress, Message: task.Message, CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt})
	}
	c.JSON(http.StatusOK, response)
}
