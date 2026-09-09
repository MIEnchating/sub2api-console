package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/gin-gonic/gin"
)

type AccountResultsLive interface {
	Subscribe([]string) (<-chan evidence.LiveUpdate, func(), error)
}
type accountResultsReader interface {
	RecentAccountResults(context.Context, string, int) ([]business.AccountRecentResult, error)
}
type accountResultEvent struct {
	AccountID string                       `json:"account_id"`
	Result    business.AccountRecentResult `json:"result"`
}
type accountResultsSnapshot struct {
	AccountID string                         `json:"account_id"`
	Results   []business.AccountRecentResult `json:"results"`
}
type accountCollectionEvent struct {
	AccountID string `json:"account_id"`
	State     string `json:"state"`
}

func parseLiveAccountIDs(raw string) ([]string, bool) {
	ids := strings.Split(raw, ",")
	if len(ids) == 0 || len(ids) > 100 {
		return nil, false
	}
	for _, id := range ids {
		if !positiveNumericID(id) {
			return nil, false
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), true
}
func (s *Server) accountResultsEvents(c *gin.Context) {
	ids, valid := parseLiveAccountIDs(c.Query("account_ids"))
	if !valid {
		writeError(c, http.StatusUnprocessableEntity, "请选择 1 至 100 个有效账号")
		return
	}
	reader, ok := s.business.(accountResultsReader)
	if !ok || s.accountResultsLive == nil {
		writeError(c, http.StatusServiceUnavailable, "实时请求采集尚未就绪")
		return
	}
	for _, id := range ids {
		account, err := s.business.Account(c.Request.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(c, http.StatusNotFound, "账号不存在，请刷新列表")
			return
		}
		if err != nil {
			writeError(c, http.StatusInternalServerError, "账号读取失败")
			return
		}
		if account == nil {
			writeError(c, http.StatusNotFound, "账号不存在，请刷新列表")
			return
		}
	}
	if !s.acquireSSESlot() {
		writeError(c, http.StatusTooManyRequests, "实时连接数量已达上限，请稍后重试")
		return
	}
	defer s.releaseSSESlot()
	updates, stop, err := s.accountResultsLive.Subscribe(ids)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "实时请求采集暂不可用，请稍后重试")
		return
	}
	defer stop()
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	controller := http.NewResponseController(c.Writer)
	send := func(event string, value any) error {
		_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if err := writeSSEEvent(c.Writer, event, value); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	previous := map[string]map[string]string{}
	refresh := func(id string, initial bool) error {
		results, err := reader.RecentAccountResults(c.Request.Context(), id, 100)
		if err != nil {
			return err
		}
		accounts := []business.AccountStatus{{RecentResults: results}}
		s.enrichRecentResults(c.Request.Context(), accounts)
		current := map[string]string{}
		for index := len(results) - 1; index >= 0; index-- {
			result := results[index]
			encoded, err := json.Marshal(result)
			if err != nil {
				return err
			}
			current[result.ID] = string(encoded)
			if !initial && previous[id][result.ID] != string(encoded) {
				if err := send("result", accountResultEvent{AccountID: id, Result: result}); err != nil {
					return err
				}
			}
		}
		previous[id] = current
		if initial {
			return send("snapshot", accountResultsSnapshot{AccountID: id, Results: results})
		}
		return nil
	}
	for _, id := range ids {
		if err := refresh(id, true); err != nil {
			return
		}
	}
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case update, open := <-updates:
			if !open {
				return
			}
			if !s.validStreamSession(c) {
				return
			}
			if err := refresh(update.AccountID, false); err != nil {
				return
			}
			state := "connected"
			if update.Failed {
				state = "retrying"
			}
			if err := send("collection", accountCollectionEvent{AccountID: update.AccountID, State: state}); err != nil {
				return
			}
		case <-ping.C:
			if !s.validStreamSession(c) {
				return
			}
			// Also include probe results and samples written by the regular inspection.
			for _, id := range ids {
				if err := refresh(id, false); err != nil {
					return
				}
			}
			if err := send("ping", struct{}{}); err != nil {
				return
			}
		}
	}
}
