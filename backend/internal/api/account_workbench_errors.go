package api

import (
	"errors"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/gin-gonic/gin"
)

func (s *Server) browserError(c *gin.Context, err error) {
	status := http.StatusConflict
	if errors.Is(err, browserlogin.ErrSession) {
		status = http.StatusNotFound
	}
	writeError(c, status, err.Error())
}
