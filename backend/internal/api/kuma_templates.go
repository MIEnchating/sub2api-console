package api

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) kumaTemplates(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	items, err := s.uptimeKuma.Templates(c.Request.Context())
	if err != nil {
		kumaError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func (s *Server) kumaTemplate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	item, err := s.uptimeKuma.Template(c.Request.Context(), c.Param("template_id"))
	if err != nil {
		kumaError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) saveKumaTemplate(c *gin.Context) {
	var in uptimekuma.TemplateInput
	if bindRequestJSON(c, &in) != nil {
		writeError(c, 422, "模板参数无效")
		return
	}
	item, err := s.uptimeKuma.SaveTemplate(c.Request.Context(), c.Param("template_id"), in)
	if err != nil {
		kumaError(c, err)
		return
	}
	s.recordRuntimeEventBestEffort(c.Request.Context(), "uptime_kuma_template", "success", "请求模板已保存", map[string]any{"template_id": item.ID, "revision": item.Revision})
	c.JSON(http.StatusOK, item)
}
func (s *Server) deleteKumaTemplate(c *gin.Context) {
	var in struct {
		Revision int64 `json:"revision"`
	}
	if bindRequestJSON(c, &in) != nil {
		writeError(c, 422, "模板版本无效")
		return
	}
	if err := s.uptimeKuma.DeleteTemplate(c.Request.Context(), c.Param("template_id"), in.Revision); err != nil {
		kumaError(c, err)
		return
	}
	s.recordRuntimeEventBestEffort(c.Request.Context(), "uptime_kuma_template", "success", "请求模板已删除", map[string]any{"template_id": c.Param("template_id")})
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) kumaTemplatePreset(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var in struct {
		Profile string `json:"request_profile"`
		Model   string `json:"model"`
	}
	if bindRequestJSON(c, &in) != nil {
		writeError(c, 422, "请求模式参数无效")
		return
	}
	item, err := uptimekuma.TemplatePreset(in.Profile, in.Model)
	if err != nil {
		kumaError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}
