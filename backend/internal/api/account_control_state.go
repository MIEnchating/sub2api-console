package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/gin-gonic/gin"
)

type accountControlStateReader interface {
	AccountControlState(context.Context, string) (*business.AccountControlState, error)
}

func (s *Server) accountControlState(c *gin.Context) {
	id := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(id) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	reader, ok := s.business.(accountControlStateReader)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "账号状态读取服务尚未就绪")
		return
	}
	var state *business.AccountControlState
	err := s.withAccountReadSnapshot(c.Request.Context(), func(ctx context.Context) error {
		var err error
		state, err = reader.AccountControlState(ctx, id)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "账号不存在")
		return
	}
	if err != nil || state == nil {
		writeError(c, http.StatusInternalServerError, "账号状态读取失败，请重试")
		return
	}
	c.JSON(http.StatusOK, state)
}
