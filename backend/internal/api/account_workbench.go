package api

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
	"net/http"
)

func workbenchOwner(c *gin.Context) string {
	token, _ := c.Cookie(sessionCookie)
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *Server) accountWorkbenchPreview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台尚未就绪")
		return
	}
	var input accountworkbench.PreviewInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "导入参数无效")
		return
	}
	result, err := s.accountWorkbench.Preview(c.Request.Context(), workbenchOwner(c), input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchTemplates(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台尚未就绪")
		return
	}
	result, err := s.accountWorkbench.Templates(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
func (s *Server) accountWorkbenchTemplateSource(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台尚未就绪")
		return
	}
	result, err := s.accountWorkbench.TemplateSource(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
func (s *Server) accountWorkbenchSaveTemplate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台尚未就绪")
		return
	}
	var input accountworkbench.TemplateInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "模板参数无效")
		return
	}
	result, err := s.accountWorkbench.SaveTemplate(c.Request.Context(), input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
func (s *Server) accountWorkbenchChangeTemplate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台尚未就绪")
		return
	}
	var input struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusUnprocessableEntity, "模板版本无效")
		return
	}
	result, err := s.accountWorkbench.ChangeTemplate(c.Request.Context(), input.ID, input.Revision, c.Request.Method == http.MethodDelete)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchAccounts(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台尚未就绪")
		return
	}
	result, err := s.accountWorkbench.Accounts(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
