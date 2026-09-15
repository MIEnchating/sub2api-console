package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchSourceProfiles(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.SourceProfiles(c.Request.Context(), owner, accountworkbench.ExportScope(c.Query("scope")))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchSourceProfileIdentity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.SourceProfileIdentityInput
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "本地资料来源无效")
		return
	}
	result, err := service.SourceProfileIdentity(c.Request.Context(), owner, input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchSaveSourceProfile(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.SourceProfileSaveInput
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "本地登录资料参数无效")
		return
	}
	result, err := service.SaveSourceProfile(c.Request.Context(), owner, input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchDeleteSourceProfile(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		Scope     accountworkbench.ExportScope `json:"scope"`
		Revision  int64                        `json:"revision"`
		Confirmed bool                         `json:"confirmed"`
	}
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "本地登录资料删除参数无效")
		return
	}
	if err := service.DeleteSourceProfile(c.Request.Context(), owner, c.Param("id"), input.Scope, input.Revision, input.Confirmed); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) accountWorkbenchSourceProfileAuthorization(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.SourceProfileSelectionInput
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "本地重新登录范围无效")
		return
	}
	result, err := service.PreviewSourceProfileAuthorization(c.Request.Context(), owner, input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchSourceProfileExportPreview(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.SourceProfileSelectionInput
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "本地资料导出范围无效")
		return
	}
	result, err := service.PreviewSourceProfileExport(c.Request.Context(), owner, input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchExportSourceProfiles(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"preview_id"`
		Confirmed bool   `json:"confirmed"`
	}
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "本地资料导出确认无效")
		return
	}
	result, err := service.ExportSourceProfiles(c.Request.Context(), owner, input.PreviewID, input.Confirmed)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (s *Server) accountWorkbenchSourceProfileSecurity(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.SourceProfileSecurityInput
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "本地资料安全结果参数无效")
		return
	}
	result, err := service.ApplySecurityToSourceProfile(c.Request.Context(), owner, c.Param("id"), input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
