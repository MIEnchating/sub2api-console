package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/gin-gonic/gin"
)

func (s *Server) cleanupUpstreamBinding(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("binding_id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, http.StatusUnprocessableEntity, business.ErrBindingCleanupInvalid.Error())
		return
	}
	var input business.UpstreamBindingCleanup
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, business.ErrBindingCleanupInvalid.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	err = s.business.CleanupUpstreamBinding(c.Request.Context(), c.Param("host"), id, input, actor)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(c, http.StatusNotFound, "绑定已不存在，请刷新列表")
	case errors.Is(err, business.ErrBindingCleanupInvalid):
		writeError(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, business.ErrBindingCleanupConflict):
		writeError(c, http.StatusConflict, err.Error())
	case err != nil:
		writeError(c, http.StatusInternalServerError, "清理失效绑定失败，请稍后重试")
	default:
		c.JSON(http.StatusOK, struct {
			BindingID int64 `json:"binding_id"`
		}{BindingID: id})
	}
}
