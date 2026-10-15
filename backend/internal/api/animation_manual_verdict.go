package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) setAnimationManualVerdict(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusGone, gin.H{"code": "animation_manual_verdict_retired", "detail": "独立手动判定已移除，请使用质量标注进行复核或纠正"})
}
