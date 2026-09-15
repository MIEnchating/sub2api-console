package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchRunPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.WorkbenchRunInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "混合输入参数无效")
		return
	}
	result, err := service.PreviewWorkbenchRun(c.Request.Context(), owner, input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDiscardRunPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	service.DeleteWorkbenchRunPreview(owner, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) accountWorkbenchStartRun(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"preview_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "混合处理确认参数无效")
		return
	}
	result, err := service.StartWorkbenchRun(c.Request.Context(), owner, input.PreviewID, input.Confirmed)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (s *Server) accountWorkbenchReadRun(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.ReadWorkbenchRun(c.Request.Context(), owner, c.Param("id"))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchRunResultPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.PreviewWorkbenchRunResult(c.Request.Context(), owner, c.Param("id"))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchCancelRun(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	if err := service.CancelWorkbenchRun(owner, c.Param("id")); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": true})
}
