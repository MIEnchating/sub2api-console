package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchResumeMixedQueue(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.QueueRecoveryAction
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "混合批次恢复参数无效")
		return
	}
	result, err := service.ResumeWorkbenchQueue(c.Request.Context(), owner, c.Param("id"), input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}
