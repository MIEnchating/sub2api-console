package api

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) accountWorkbenchQueueRecoveries(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.QueueRecoveries(c.Request.Context(), owner, accountworkbench.ExportScope(c.Query("scope")))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
func (s *Server) accountWorkbenchResumeOAuthQueue(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.QueueRecoveryAction
	if bindRequestJSON(c, &input) != nil {
		writeError(c, 422, "批量恢复参数无效")
		return
	}
	result, err := service.ResumeOAuthQueue(c.Request.Context(), owner, c.Param("id"), input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}
func (s *Server) accountWorkbenchDeleteQueueRecovery(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.QueueRecoveryAction
	if bindRequestJSON(c, &input) != nil {
		writeError(c, 422, "删除批量恢复资料参数无效")
		return
	}
	if err := service.DeleteQueueRecovery(c.Request.Context(), owner, c.Param("id"), input); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
