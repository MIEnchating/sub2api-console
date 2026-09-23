package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"github.com/gin-gonic/gin"
)

func (s *Server) newAPIChannels(c *gin.Context) {
	manager := s.newAPIManagement
	if manager == nil {
		writeError(c, http.StatusServiceUnavailable, "渠道管理服务不可用")
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "0"))
	if err != nil || page < 0 || page > 999 {
		writeError(c, http.StatusBadRequest, "渠道页码无效")
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		writeError(c, http.StatusBadRequest, "每页行数必须为 1 到 100")
		return
	}
	var result newapimanagement.ChannelPage
	if groupID := c.Query("group_id"); groupID != "" {
		groups, readErr := s.private.NewAPIChannelGroups(c.Request.Context(), c.Param("platform_id"))
		if readErr != nil {
			writeError(c, http.StatusBadRequest, "渠道分组读取失败")
			return
		}
		found := false
		for _, group := range groups.Groups {
			if group.ID == groupID {
				found = true
				result, err = manager.ChannelsByID(c.Request.Context(), c.Param("platform_id"), group.ChannelIDs, page, pageSize)
				break
			}
		}
		if !found {
			writeError(c, http.StatusNotFound, "渠道分组不存在，请刷新分组")
			return
		}
	} else {
		result, err = manager.Channels(c.Request.Context(), c.Param("platform_id"), page, pageSize)
	}
	if err != nil {
		writeNewAPIError(c, err, http.StatusBadGateway)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) availableNewAPIChannelModels(c *gin.Context) {
	if s.newAPIManagement == nil {
		writeError(c, http.StatusServiceUnavailable, "渠道管理服务不可用")
		return
	}
	models, err := s.newAPIManagement.AvailableChannelModels(c.Request.Context(), c.Param("platform_id"), c.Param("channel_id"), c.Query("version"))
	if err != nil {
		writeNewAPIError(c, err, http.StatusBadGateway)
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": models})
}

func (s *Server) changeNewAPIChannelModels(c *gin.Context) {
	manager := s.newAPIManagement
	if manager == nil {
		writeError(c, http.StatusServiceUnavailable, "渠道管理服务不可用")
		return
	}
	var input newapimanagement.ChannelModelChange
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusBadRequest, "请选择有效模型和渠道版本")
		return
	}
	result, err := manager.ChangeChannelModels(c.Request.Context(), c.Param("platform_id"), c.Param("channel_id"), input)
	if err != nil {
		writeNewAPIError(c, err, http.StatusBadGateway)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) batchNewAPIChannelModels(c *gin.Context) {
	if s.newAPIChannelTasks == nil {
		writeError(c, http.StatusServiceUnavailable, "渠道任务服务不可用")
		return
	}
	var input newapimanagement.ChannelBatchInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusBadRequest, "请选择最多 50 个渠道、有效模型及渠道版本")
		return
	}
	task, err := s.newAPIChannelTasks.Enqueue(c.Request.Context(), c.Param("platform_id"), input)
	if err != nil {
		writeNewAPIError(c, err, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusAccepted, task)
}

type newAPIChannelGroupsInput struct {
	Groups  []configstore.NewAPIChannelGroup `json:"groups" binding:"required"`
	Version string                           `json:"version"`
}

func (s *Server) newAPIChannelGroups(c *gin.Context) {
	platform, err := s.private.NewAPIPlatform(c.Request.Context(), c.Param("platform_id"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "平台配置读取失败")
		return
	}
	if platform == nil {
		writeError(c, http.StatusNotFound, "平台不存在")
		return
	}
	groups, err := s.private.NewAPIChannelGroups(c.Request.Context(), c.Param("platform_id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, groups)
}
func (s *Server) saveNewAPIChannelGroups(c *gin.Context) {
	platform, err := s.private.NewAPIPlatform(c.Request.Context(), c.Param("platform_id"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "平台配置读取失败")
		return
	}
	if platform == nil {
		writeError(c, http.StatusNotFound, "平台不存在")
		return
	}
	var input newAPIChannelGroupsInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusBadRequest, "渠道分组请求无效")
		return
	}
	groups, err := s.private.SaveNewAPIChannelGroups(c.Request.Context(), c.Param("platform_id"), input.Groups, input.Version)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, configstore.ErrChannelGroupsConflict) {
			status = http.StatusConflict
		}
		writeError(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, groups)
}
