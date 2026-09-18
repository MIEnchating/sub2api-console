package api

import (
	"encoding/base64"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/gin-gonic/gin"
)

func (s *Server) workbenchReady(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	if s.accountWorkbench == nil {
		writeError(c, http.StatusServiceUnavailable, "账号工作台尚未就绪")
		return false
	}
	return true
}
func (s *Server) accountWorkbenchRuns(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	result, err := s.accountWorkbench.Runs(c.Request.Context(), workbenchOwner(c))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
func (s *Server) accountWorkbenchRun(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	result, err := s.accountWorkbench.Run(c.Request.Context(), workbenchOwner(c), c.Param("id"))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
func (s *Server) accountWorkbenchStart(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	var input accountworkbench.RunConfirmation
	if bindRequestJSON(c, &input) != nil {
		writeError(c, http.StatusUnprocessableEntity, "请先预览并确认本批账号")
		return
	}
	result, err := s.accountWorkbench.Start(c.Request.Context(), workbenchOwner(c), input)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, result)
}
func (s *Server) accountWorkbenchRunAction(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	var body struct {
		Revision int64    `json:"revision"`
		IDs      []string `json:"ids"`
	}
	if bindRequestJSON(c, &body) != nil {
		writeError(c, http.StatusUnprocessableEntity, "记录版本或所选账号无效")
		return
	}
	input := accountworkbench.RunConfirmation{ID: c.Param("id"), Revision: body.Revision}
	ctx, owner := c.Request.Context(), workbenchOwner(c)
	var result any
	var err error
	status := http.StatusOK
	switch c.Param("action") {
	case "retry":
		result, err = s.accountWorkbench.Retry(ctx, owner, input)
		status = http.StatusAccepted
	case "enable":
		result, err = s.accountWorkbench.EnableReview(ctx, owner, input, body.IDs)
		status = http.StatusAccepted
	case "export":
		result, err = s.accountWorkbench.Export(ctx, owner, input)
	case "cancel":
		err = s.accountWorkbench.Cancel(ctx, owner, input)
		result = gin.H{"cancelled": err == nil}
	case "delete":
		err = s.accountWorkbench.Delete(ctx, owner, input)
		result = gin.H{"deleted": err == nil}
	default:
		writeError(c, http.StatusNotFound, "不支持此工作台操作")
		return
	}
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(status, result)
}
func (s *Server) accountWorkbenchBrowser(c *gin.Context) {
	if !s.workbenchReady(c) {
		return
	}
	ctx, owner := c.Request.Context(), workbenchOwner(c)
	if c.Request.Method == http.MethodPost {
		var input browserlogin.Input
		if bindRequestJSON(c, &input) != nil {
			writeError(c, http.StatusUnprocessableEntity, "浏览器操作无效")
			return
		}
		if err := s.accountWorkbench.BrowserInput(ctx, owner, c.Param("id"), c.Param("item"), input); err != nil {
			writeError(c, http.StatusConflict, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"accepted": true})
		return
	}
	image, err := s.accountWorkbench.BrowserImage(ctx, owner, c.Param("id"), c.Param("item"))
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(image), "width": browserlogin.Width, "height": browserlogin.Height})
}
