package api

import (
	"net/http"
	"strconv"

	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
	"github.com/gin-gonic/gin"
)

func kumaError(c *gin.Context, err error) {
	e := uptimekuma.PublicError(err)
	c.JSON(e.Status, gin.H{"code": e.Code, "detail": e.Message})
}
func (s *Server) kumaConfig(c *gin.Context) {
	r, err := s.uptimeKuma.Config(c.Request.Context())
	if err != nil {
		kumaError(c, err)
		return
	}
	c.JSON(http.StatusOK, r)
}
func (s *Server) saveKumaConfig(c *gin.Context) {
	var in uptimekuma.ConfigInput
	if bindRequestJSON(c, &in) != nil {
		writeError(c, 422, "接入配置参数无效")
		return
	}
	if s.kumaTasks == nil {
		writeError(c, 503, "Uptime Kuma 任务服务尚未就绪")
		return
	}
	r, err := s.kumaTasks.Save(c.Request.Context(), in)
	if err != nil {
		kumaError(c, err)
		return
	}
	s.recordRuntimeEventBestEffort(c.Request.Context(), "uptime_kuma_config", "success", "Uptime Kuma 接入配置任务已创建", map[string]any{"task_id": r.ID})
	c.JSON(http.StatusAccepted, r)
}
func (s *Server) disconnectKuma(c *gin.Context) {
	var in struct {
		Revision int64 `json:"revision"`
	}
	if bindRequestJSON(c, &in) != nil {
		writeError(c, 422, "接入配置版本无效")
		return
	}
	if err := s.uptimeKuma.Disconnect(c.Request.Context(), in.Revision); err != nil {
		kumaError(c, err)
		return
	}
	s.recordRuntimeEventBestEffort(c.Request.Context(), "uptime_kuma_config", "success", "Uptime Kuma 接入已断开", nil)
	c.JSON(http.StatusOK, gin.H{"disconnected": true})
}
func (s *Server) kumaMonitors(c *gin.Context) {
	r, err := s.uptimeKuma.Snapshot(c.Request.Context())
	if err != nil {
		kumaError(c, err)
		return
	}
	c.JSON(http.StatusOK, r)
}
func (s *Server) kumaPushURL(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseInt(c.Param("monitor_id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, 422, "监控项 ID 无效")
		return
	}
	value, err := s.uptimeKuma.PushURL(c.Request.Context(), id, c.Query("revision"))
	if err != nil {
		kumaError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": value})
}
func (s *Server) writeKumaMonitor(c *gin.Context) {
	var in uptimekuma.WriteInput
	if bindRequestJSON(c, &in) != nil {
		writeError(c, 422, "监控操作参数无效")
		return
	}
	id := int64(0)
	if raw := c.Param("monitor_id"); raw != "" {
		var err error
		id, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeError(c, 422, "监控项 ID 无效")
			return
		}
	}
	if id == 0 {
		in.Action = "create"
	}
	if s.kumaTasks == nil {
		writeError(c, 503, "Uptime Kuma 任务服务尚未就绪")
		return
	}
	task, err := s.kumaTasks.Write(c.Request.Context(), id, in)
	status := "success"
	if err != nil {
		status = "failed"
	}
	s.recordRuntimeEventBestEffort(c.Request.Context(), "uptime_kuma_monitor", status, "Uptime Kuma 监控操作任务创建", map[string]any{"monitor_id": id, "action": in.Action, "task_id": task.ID})
	if err != nil {
		kumaError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, task)
}

func (s *Server) kumaResources(c *gin.Context) {
	if c.Param("kind") != "status-pages" {
		writeError(c, 404, "此管理功能已移除")
		return
	}
	r, e := s.uptimeKuma.Resources(c.Request.Context(), c.Param("kind"))
	if e != nil {
		kumaError(c, e)
		return
	}
	c.JSON(http.StatusOK, r)
}
func (s *Server) kumaResource(c *gin.Context) {
	if c.Param("kind") != "status-pages" {
		writeError(c, 404, "此管理功能已移除")
		return
	}
	id, e := strconv.ParseInt(c.Param("resource_id"), 10, 64)
	if e != nil || id <= 0 {
		writeError(c, 422, "资源 ID 无效")
		return
	}
	r, e := s.uptimeKuma.Resource(c.Request.Context(), c.Param("kind"), id)
	if e != nil {
		kumaError(c, e)
		return
	}
	c.JSON(http.StatusOK, r)
}
func (s *Server) writeKumaResource(c *gin.Context) {
	if c.Param("kind") != "status-pages" {
		writeError(c, 404, "此管理功能已移除")
		return
	}
	id, e := strconv.ParseInt(c.Param("resource_id"), 10, 64)
	if e != nil || id < 0 {
		writeError(c, 422, "资源 ID 无效")
		return
	}
	var in uptimekuma.ResourceInput
	if bindRequestJSON(c, &in) != nil {
		writeError(c, 422, "资源参数无效")
		return
	}
	if s.kumaTasks == nil {
		writeError(c, 503, "Uptime Kuma 任务服务尚未就绪")
		return
	}
	task, e := s.kumaTasks.WriteResource(c.Request.Context(), c.Param("kind"), id, in)
	if e != nil {
		kumaError(c, e)
		return
	}
	s.recordRuntimeEventBestEffort(c.Request.Context(), "uptime_kuma_resource", "success", "Uptime Kuma 管理任务已创建", map[string]any{"kind": c.Param("kind"), "resource_id": id, "action": in.Action, "task_id": task.ID})
	c.JSON(http.StatusAccepted, task)
}
