package api

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamexclusion"
	"github.com/gin-gonic/gin"
	"net/http"
)

type UpstreamExclusionService interface {
	Preview(context.Context, string) (business.UpstreamExclusionPreview, error)
	Enqueue(context.Context, string, upstreamexclusion.Request, string) (taskstore.Task, error)
}

func (s *Server) upstreamExclusionPreview(c *gin.Context) {
	if s.upstreamExclusion == nil {
		writeError(c, http.StatusServiceUnavailable, "上游排除服务尚未就绪")
		return
	}
	preview, err := s.upstreamExclusion.Preview(c.Request.Context(), c.Param("host"))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, preview)
}
func (s *Server) setUpstreamExclusion(c *gin.Context) {
	if s.upstreamExclusion == nil {
		writeError(c, http.StatusServiceUnavailable, "上游排除服务尚未就绪")
		return
	}
	var input struct {
		UpstreamID         string   `json:"upstream_id"`
		Excluded           *bool    `json:"excluded"`
		StopScheduling     *bool    `json:"stop_scheduling"`
		ExpectedAccountIDs []string `json:"expected_account_ids"`
	}
	if err := bindRequestJSON(c, &input); err != nil || input.UpstreamID == "" || input.Excluded == nil || input.StopScheduling == nil || input.ExpectedAccountIDs == nil || len(input.ExpectedAccountIDs) > 100000 {
		writeError(c, http.StatusUnprocessableEntity, "请提供稳定上游 ID、排除选项、调度选项及已确认账号范围")
		return
	}
	seen := map[string]bool{}
	for _, id := range input.ExpectedAccountIDs {
		if !positiveNumericID(id) || seen[id] {
			writeError(c, http.StatusUnprocessableEntity, "账号范围必须包含不重复的稳定数字 ID")
			return
		}
		seen[id] = true
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.upstreamExclusion.Enqueue(c.Request.Context(), c.Param("host"), upstreamexclusion.Request{UpstreamID: input.UpstreamID, Excluded: *input.Excluded, StopScheduling: *input.StopScheduling, ExpectedAccountIDs: input.ExpectedAccountIDs}, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}
