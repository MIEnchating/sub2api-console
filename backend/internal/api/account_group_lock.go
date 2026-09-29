package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type accountGroupLockStore interface {
	SetAccountGroupsLocked(context.Context, string, bool, string) error
}

func (s *Server) setAccountGroupsLocked(c *gin.Context) {
	accountID := c.Param("account_id")
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	payload, err := decodeRequestObject(c)
	enabled, valid := payload["groups_locked"].(bool)
	if err != nil || len(payload) != 1 || !valid {
		writeError(c, http.StatusUnprocessableEntity, "参数必须只包含布尔值 groups_locked")
		return
	}
	if _, ok := s.accountMutationPreflight(c, accountID, false); !ok {
		return
	}
	repository, ok := s.business.(accountGroupLockStore)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "分组锁定服务尚未就绪")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	if err := repository.SetAccountGroupsLocked(c.Request.Context(), accountID, enabled, actor); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups_locked": enabled})
}
