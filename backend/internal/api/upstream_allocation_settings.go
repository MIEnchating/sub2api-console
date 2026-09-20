package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/gin-gonic/gin"
)

func allocationSettingError(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "账号或上游不存在，请刷新列表")
		return
	}
	writeError(c, http.StatusConflict, err.Error())
}

func (s *Server) getUpstreamAllocationSetting(c *gin.Context) {
	result, err := s.business.UpstreamAllocationSetting(c.Request.Context(), c.Param("kind"), c.Param("id"))
	if err != nil {
		allocationSettingError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) setUpstreamAllocationSetting(c *gin.Context) {
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "共享并发设置必须是对象")
		return
	}
	for key := range payload {
		if key != "override" && key != "expected_revision" && key != "expected_upstream_id" {
			writeError(c, http.StatusUnprocessableEntity, "共享并发设置包含未知字段")
			return
		}
	}
	revision, ok := payload["expected_revision"].(string)
	if !ok || strings.TrimSpace(revision) == "" {
		writeError(c, http.StatusUnprocessableEntity, "缺少策略版本，请刷新后重试")
		return
	}
	update := business.UpstreamAllocationUpdate{ExpectedRevision: revision}
	raw, present := payload["override"]
	if !present {
		writeError(c, http.StatusUnprocessableEntity, "必须指定 override，恢复跟随请使用 null")
		return
	}
	if raw != nil {
		value, ok := raw.(bool)
		if !ok {
			writeError(c, http.StatusUnprocessableEntity, "override 必须是布尔值或 null")
			return
		}
		update.Override = &value
	}
	if raw, present := payload["expected_upstream_id"]; present {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			writeError(c, http.StatusUnprocessableEntity, "上游稳定 ID 无效")
			return
		}
		update.ExpectedUpstreamID = value
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	result, err := s.business.SetUpstreamAllocationSetting(c.Request.Context(), c.Param("kind"), c.Param("id"), update, actor)
	if err != nil {
		allocationSettingError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
