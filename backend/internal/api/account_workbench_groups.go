package api

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) accountWorkbenchGroups(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !positiveNumericID(c.Param("id")) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台尚未就绪")
		return
	}
	result, err := s.accountWorkbench.AccountGroups(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) moveWorkbenchAccountGroups(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !positiveNumericID(c.Param("id")) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台尚未就绪")
		return
	}
	var input accountworkbench.MoveGroupsInput
	if err := bindRequestJSON(c, &input); err != nil || input.ExpectedGroupIDs == nil || input.TargetVersion == "" || input.AccountVersion == "" || len(input.GroupIDs) < 1 || len(input.GroupIDs) > 50 {
		writeError(c, http.StatusUnprocessableEntity, "请提供账号版本、原分组和 1 到 50 个目标分组")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.accountWorkbench.MoveAccountGroups(c.Request.Context(), c.Param("id"), input, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}
