package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/gin-gonic/gin"
)

type browserLoginService interface {
	StartBrowserLogin(context.Context, string, string, string) (browserlogin.View, error)
	ReadBrowserLogin(context.Context, string, string) (browserlogin.View, error)
	InputBrowserLogin(context.Context, string, string, browserlogin.Input) error
	FinishBrowserLogin(string, string) error
	CancelBrowserLogin(string, string) error
}

func (s *Server) browserOwner(c *gin.Context) (browserLoginService, string, bool) {
	c.Header("Cache-Control", "no-store")
	service, ok := s.authRecovery.(browserLoginService)
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "浏览器验证服务尚未就绪")
		return nil, "", false
	}
	user, sessionErr := s.sessionUser(c)
	if sessionErr != nil || user == nil {
		writeError(c, http.StatusForbidden, "浏览器验证需要先登录控制台")
		return nil, "", false
	}
	token, err := c.Cookie(sessionCookie)
	if err != nil || token == "" {
		writeError(c, http.StatusForbidden, "浏览器验证需要先登录控制台")
		return nil, "", false
	}
	hash := sha256.Sum256([]byte(token))
	return service, hex.EncodeToString(hash[:]), true
}
func (s *Server) startBrowserLogin(c *gin.Context) {
	service, owner, ok := s.browserOwner(c)
	if !ok {
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil || len(payload) != 1 {
		writeError(c, 422, "浏览器验证参数只能包含上游 Host")
		return
	}
	host, err := requiredText(payload, "host", 1, 512)
	if err != nil {
		writeError(c, 422, err.Error())
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, 500, "控制台会话读取失败")
		return
	}
	result, err := service.StartBrowserLogin(c.Request.Context(), owner, host, actor)
	if err != nil {
		writeError(c, 409, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, result)
}
func (s *Server) readBrowserLogin(c *gin.Context) {
	service, owner, ok := s.browserOwner(c)
	if !ok {
		return
	}
	result, err := service.ReadBrowserLogin(c.Request.Context(), owner, c.Param("id"))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(200, result)
}
func (s *Server) inputBrowserLogin(c *gin.Context) {
	service, owner, ok := s.browserOwner(c)
	if !ok {
		return
	}
	var payload browserlogin.Input
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeError(c, 422, "浏览器操作参数无效")
		return
	}
	if err := payload.Validate(); err != nil {
		writeError(c, 422, err.Error())
		return
	}
	if err := service.InputBrowserLogin(c.Request.Context(), owner, c.Param("id"), payload); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(200, gin.H{"accepted": true})
}
func (s *Server) finishBrowserLogin(c *gin.Context) {
	service, owner, ok := s.browserOwner(c)
	if !ok {
		return
	}
	if err := service.FinishBrowserLogin(owner, c.Param("id")); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(202, gin.H{"accepted": true})
}
func (s *Server) cancelBrowserLogin(c *gin.Context) {
	service, owner, ok := s.browserOwner(c)
	if !ok {
		return
	}
	if err := service.CancelBrowserLogin(owner, c.Param("id")); err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(200, gin.H{"cancelled": true})
}
func (s *Server) browserError(c *gin.Context, err error) {
	status := http.StatusConflict
	if errors.Is(err, browserlogin.ErrSession) {
		status = http.StatusNotFound
	}
	writeError(c, status, err.Error())
}
