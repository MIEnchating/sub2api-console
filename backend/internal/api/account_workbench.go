package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/gin-gonic/gin"
)

func (s *Server) workbenchService(c *gin.Context) (*accountworkbench.Service, string, bool) {
	c.Header("Cache-Control", "no-store")
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台服务尚未就绪")
		return nil, "", false
	}
	user, sessionErr := s.sessionUser(c)
	if sessionErr != nil || user == nil {
		writeError(c, http.StatusForbidden, "账号工作台需要先登录控制台")
		return nil, "", false
	}
	token, err := c.Cookie(sessionCookie)
	if err != nil || strings.TrimSpace(token) == "" {
		writeError(c, http.StatusForbidden, "账号工作台需要先登录控制台")
		return nil, "", false
	}
	digest := sha256.Sum256([]byte(token))
	return s.accountWorkbench, hex.EncodeToString(digest[:]), true
}

func (s *Server) accountWorkbenchTemplates(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := service.Templates(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchSaveTemplate(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.TemplateInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "模板参数无效")
		return
	}
	id := c.Param("template_id")
	result, err := service.SaveTemplate(c.Request.Context(), id, input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDeleteTemplate(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "模板版本无效")
		return
	}
	if err := service.DeleteTemplate(c.Request.Context(), c.Param("template_id"), input.Revision); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) accountWorkbenchPreferTemplate(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		Revision  int64 `json:"revision"`
		Preferred *bool `json:"preferred"`
	}
	if err := bindRequestJSON(c, &input); err != nil || input.Preferred == nil || input.Revision <= 0 {
		writeError(c, http.StatusUnprocessableEntity, "首选模板参数或版本无效")
		return
	}
	result, err := service.SetPreferredTemplate(c.Request.Context(), c.Param("template_id"), input.Revision, *input.Preferred)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchTemplateFromAccount(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		AccountID string `json:"account_id"`
	}
	if err := bindRequestJSON(c, &input); err != nil || strings.TrimSpace(input.AccountID) == "" {
		writeError(c, http.StatusUnprocessableEntity, "账号 ID 无效")
		return
	}
	result, err := service.TemplateFromAccount(c.Request.Context(), strings.TrimSpace(input.AccountID))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.PreviewInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "账号输入参数无效")
		return
	}
	result, err := service.Preview(c.Request.Context(), owner, input)
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDeletePreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	service.DeletePreview(owner, c.Param("preview_id"))
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) accountWorkbenchImport(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"preview_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "导入确认参数无效")
		return
	}
	task, err := service.Import(c.Request.Context(), owner, input.PreviewID, input.Confirmed)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, task)
}

func (s *Server) accountWorkbenchHistory(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.History(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "账号工作台历史读取失败")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchMaintenance(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.Maintenance(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchSaveMaintenance(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input configstore.WorkbenchMaintenance
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "维护配置无效")
		return
	}
	result, err := service.SaveMaintenance(c.Request.Context(), input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	if err := service.BindMaintenanceOwner(c.Request.Context(), owner, result.Revision); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	result, err = service.Maintenance(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, "维护配置已保存，请刷新读取当前授权状态")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchCheckMaintenance(c *gin.Context) {
	service, _, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		Confirmed bool  `json:"confirmed"`
		Revision  int64 `json:"revision"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "维护确认参数无效")
		return
	}
	task, err := service.CheckMaintenance(c.Request.Context(), input.Revision, input.Confirmed)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, task)
}
