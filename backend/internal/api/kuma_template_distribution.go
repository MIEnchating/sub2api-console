package api

import (
	"net/http"
	"strconv"

	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
	"github.com/gin-gonic/gin"
)

func (s *Server) previewKumaTemplateDistribution(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	revision, err := strconv.ParseInt(c.Query("revision"), 10, 64)
	if err != nil || revision < 1 {
		writeError(c, 422, "模板版本无效，请刷新模板列表")
		return
	}
	preview, err := s.uptimeKuma.PreviewTemplateDistribution(c.Request.Context(), c.Param("template_id"), revision)
	if err != nil {
		kumaError(c, err)
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (s *Server) distributeKumaTemplate(c *gin.Context) {
	var input uptimekuma.TemplateDistributionInput
	if bindRequestJSON(c, &input) != nil {
		writeError(c, 422, "参数下发请求无效，请重新预览")
		return
	}
	if s.kumaTasks == nil {
		writeError(c, 503, "Uptime Kuma 任务服务尚未就绪")
		return
	}
	task, err := s.kumaTasks.DistributeTemplate(c.Request.Context(), c.Param("template_id"), input)
	if err != nil {
		kumaError(c, err)
		return
	}
	s.recordRuntimeEventBestEffort(c.Request.Context(), "uptime_kuma_template", "success", "模板参数下发任务已创建", map[string]any{"task_id": task.ID, "template_id": c.Param("template_id"), "revision": input.Revision})
	c.JSON(http.StatusAccepted, task)
}
