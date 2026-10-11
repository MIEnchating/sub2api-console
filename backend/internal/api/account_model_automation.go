package api

import (
	"net/http"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/gin-gonic/gin"
)

type accountModelAutomationRequest struct {
	GroupIDs       []string                          `json:"group_ids" binding:"required,min=1,max=50,dive,required"`
	PlatformID     string                            `json:"platform_id" binding:"required,max=128"`
	Channels       []accountautomation.ChannelTarget `json:"channels" binding:"max=50,dive"`
	SelectedModels []string                          `json:"selected_models" binding:"max=1000,dive,required,max=255"`
	PublishMissing bool                              `json:"publish_missing"`
	PriceEnabled   bool                              `json:"price_enabled"`
	PricingSource  string                            `json:"pricing_source" binding:"omitempty,oneof=official cached"`
	DryRun         bool                              `json:"dry_run"`
	ConfigVersion  int                               `json:"config_version" binding:"omitempty,min=1"`
	PreviewTaskID  string                            `json:"preview_task_id" binding:"omitempty,len=32"`
}

type accountModelAutomationScheduleRequest struct {
	ID              string                            `json:"id"`
	Name            string                            `json:"name" binding:"required,max=128"`
	GroupIDs        []string                          `json:"group_ids" binding:"required,min=1,max=50,dive,required"`
	PlatformID      string                            `json:"platform_id" binding:"required,max=128"`
	Channels        []accountautomation.ChannelTarget `json:"channels" binding:"max=50,dive"`
	SelectedModels  []string                          `json:"selected_models" binding:"max=1000,dive,required,max=255"`
	PublishMissing  bool                              `json:"publish_missing"`
	PriceEnabled    bool                              `json:"price_enabled"`
	PricingSource   string                            `json:"pricing_source" binding:"omitempty,oneof=official cached"`
	Enabled         bool                              `json:"enabled"`
	ScheduleType    string                            `json:"schedule_type" binding:"omitempty,oneof=interval daily"`
	DailyTimes      []string                          `json:"daily_times" binding:"max=24,dive"`
	Timezone        string                            `json:"timezone" binding:"omitempty,eq=Asia/Shanghai"`
	IntervalMinutes int                               `json:"interval_minutes"`
}

type accountModelAutomationTaskGroupRequest struct {
	ID          string   `json:"id" binding:"omitempty,len=32"`
	Name        string   `json:"name" binding:"required,max=128"`
	ScheduleIDs []string `json:"schedule_ids" binding:"required,min=2,max=50,dive,len=32"`
}

func (s *Server) enqueueAccountModelAutomation(c *gin.Context) {
	if s.accountModelAutomation == nil {
		writeError(c, http.StatusServiceUnavailable, "账号模型服务不可用")
		return
	}
	var payload accountModelAutomationRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusBadRequest, "请选择有效分组和平台；如选择渠道，请确认渠道版本")
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	task, err := s.accountModelAutomation.Enqueue(c.Request.Context(), accountautomation.Request{
		GroupIDs: payload.GroupIDs, PlatformID: payload.PlatformID, Channels: payload.Channels,
		SelectedModels: payload.SelectedModels, PublishMissing: payload.PublishMissing, PriceEnabled: payload.PriceEnabled,
		PricingSource: payload.PricingSource, DryRun: payload.DryRun, ConfigVersion: payload.ConfigVersion, PreviewTaskID: payload.PreviewTaskID, Actor: actor,
	})
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) accountModelAutomationSchedules(c *gin.Context) {
	if s.accountModelAutomation == nil {
		writeError(c, http.StatusServiceUnavailable, "账号模型服务不可用")
		return
	}
	items, err := s.accountModelAutomation.Schedules(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) saveAccountModelAutomationSchedule(c *gin.Context) {
	if s.accountModelAutomation == nil {
		writeError(c, http.StatusServiceUnavailable, "账号模型服务不可用")
		return
	}
	var payload accountModelAutomationScheduleRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusBadRequest, "请填写有效任务名称、运行间隔并选择分组和平台；渠道可选")
		return
	}
	if strings.TrimSpace(payload.PricingSource) == "" {
		payload.PricingSource = "official"
	}
	schedule, err := s.accountModelAutomation.SaveSchedule(c.Request.Context(), accountautomation.Schedule{
		ID: payload.ID, Name: strings.TrimSpace(payload.Name), Enabled: payload.Enabled, ScheduleType: payload.ScheduleType, DailyTimes: payload.DailyTimes, Timezone: payload.Timezone, IntervalMinutes: payload.IntervalMinutes,
		Request: accountautomation.Request{GroupIDs: payload.GroupIDs, PlatformID: payload.PlatformID, Channels: payload.Channels, SelectedModels: payload.SelectedModels, PublishMissing: payload.PublishMissing, PriceEnabled: payload.PriceEnabled, PricingSource: payload.PricingSource},
	})
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, schedule)
}

func (s *Server) deleteAccountModelAutomationSchedule(c *gin.Context) {
	if s.accountModelAutomation == nil {
		writeError(c, http.StatusServiceUnavailable, "账号模型服务不可用")
		return
	}
	if err := s.accountModelAutomation.DeleteSchedule(c.Request.Context(), c.Param("schedule_id")); err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) runAccountModelAutomationSchedule(c *gin.Context) {
	if s.accountModelAutomation == nil {
		writeError(c, http.StatusServiceUnavailable, "账号模型服务不可用")
		return
	}
	task, err := s.accountModelAutomation.RunSchedule(c.Request.Context(), c.Param("schedule_id"))
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) accountModelAutomationTaskGroups(c *gin.Context) {
	if s.accountModelAutomation == nil {
		writeError(c, http.StatusServiceUnavailable, "账号模型服务不可用")
		return
	}
	groups, err := s.accountModelAutomation.TaskGroups(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, groups)
}

func (s *Server) saveAccountModelAutomationTaskGroup(c *gin.Context) {
	if s.accountModelAutomation == nil {
		writeError(c, http.StatusServiceUnavailable, "账号模型服务不可用")
		return
	}
	var payload accountModelAutomationTaskGroupRequest
	if err := bindRequestJSON(c, &payload); err != nil {
		writeError(c, http.StatusBadRequest, "请填写任务组名称并选择至少两个运营任务")
		return
	}
	group, err := s.accountModelAutomation.SaveTaskGroup(c.Request.Context(), accountautomation.TaskGroup{
		ID: payload.ID, Name: strings.TrimSpace(payload.Name), ScheduleIDs: payload.ScheduleIDs,
	})
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, group)
}

func (s *Server) deleteAccountModelAutomationTaskGroup(c *gin.Context) {
	if s.accountModelAutomation == nil {
		writeError(c, http.StatusServiceUnavailable, "账号模型服务不可用")
		return
	}
	if err := s.accountModelAutomation.DeleteTaskGroup(c.Request.Context(), c.Param("group_id")); err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) runAccountModelAutomationTaskGroup(c *gin.Context) {
	if s.accountModelAutomation == nil {
		writeError(c, http.StatusServiceUnavailable, "账号模型服务不可用")
		return
	}
	task, err := s.accountModelAutomation.RunTaskGroup(c.Request.Context(), c.Param("group_id"))
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusOK, task)
}
