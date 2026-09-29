package api

import (
	"context"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/gin-gonic/gin"
)

type accountGroupsService interface {
	AccountGroups(context.Context, string) (accountops.AccountGroupsPreview, error)
	EnqueueGroups(context.Context, string, accountops.AccountGroupsInput, string) (taskstore.Task, error)
}

func (s *Server) accountGroups(c *gin.Context) {
	if !positiveNumericID(c.Param("account_id")) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	service, ok := s.accountTasks.(accountGroupsService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "账号分组服务尚未就绪")
		return
	}
	preview, err := service.AccountGroups(c.Request.Context(), c.Param("account_id"))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (s *Server) updateAccountGroups(c *gin.Context) {
	if !positiveNumericID(c.Param("account_id")) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	var input accountops.AccountGroupsInput
	if err := c.ShouldBindJSON(&input); err != nil || input.ExpectedGroupIDs == nil || input.TargetVersion == "" || len(input.GroupIDs) == 0 || len(input.GroupIDs) > 50 {
		writeError(c, http.StatusUnprocessableEntity, "请提供原分组、管理目标版本和 1 到 50 个目标分组")
		return
	}
	if _, ok := s.accountMutationPreflight(c, c.Param("account_id"), true); !ok {
		return
	}
	service, ok := s.accountTasks.(accountGroupsService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "账号分组服务尚未就绪")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := service.EnqueueGroups(c.Request.Context(), c.Param("account_id"), input, actor)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}
