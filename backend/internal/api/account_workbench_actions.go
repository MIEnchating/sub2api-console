package api

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) accountWorkbenchActionPreview(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	if !positiveNumericID(c.Param("id")) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	result, err := s.accountWorkbench.AccountActionPreview(c.Request.Context(), workbenchOwner(c), c.Param("id"))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchAction(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	var input accountworkbench.AccountActionInput
	if !positiveNumericID(c.Param("id")) || bindRequestJSON(c, &input) != nil || input.TargetVersion == "" || input.AccountVersion == "" || (input.Action != "delete" && input.Action != "scheduling") || (input.Action == "scheduling" && input.Schedulable == nil) {
		writeError(c, http.StatusUnprocessableEntity, "请提供有效账号 ID、操作及确认版本")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.accountWorkbench.StartAccountAction(c.Request.Context(), workbenchOwner(c), c.Param("id"), input, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}
