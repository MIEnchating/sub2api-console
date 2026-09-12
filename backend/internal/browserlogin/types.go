// Package browserlogin provides short-lived, operator-controlled login browsers.
// Only screenshots and bounded input commands cross the console API boundary.
package browserlogin

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const Width = 1100
const Height = 760
const Lifetime = 15 * time.Minute

var ErrSession = errors.New("浏览器验证会话不存在、已过期或不属于当前登录会话")

type View struct {
	ID        string `json:"id"`
	TaskID    string `json:"task_id"`
	Host      string `json:"host"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	ExpiresAt string `json:"expires_at"`
	Image     string `json:"image,omitempty"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type Input struct {
	Kind  string  `json:"kind"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Delta float64 `json:"delta"`
	Text  string  `json:"text"`
	Key   string  `json:"key"`
	Shift bool    `json:"shift"`
}

func (v Input) Validate() error {
	finite := func(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }
	switch v.Kind {
	case "click":
		if finite(v.X) && finite(v.Y) && v.X >= 0 && v.X < Width && v.Y >= 0 && v.Y < Height {
			return nil
		}
	case "scroll":
		if finite(v.Delta) && math.Abs(v.Delta) <= 1500 {
			return nil
		}
	case "text":
		if len(v.Text) > 0 && len(v.Text) <= 4096 {
			return nil
		}
	case "key":
		switch v.Key {
		case "Tab", "Enter", "Backspace", "Delete", "Escape", "ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End":
			return nil
		}
	}
	return errors.New("浏览器操作参数无效")
}

type Browser interface {
	Screenshot(context.Context) ([]byte, error)
	Input(context.Context, Input) error
	Credentials(context.Context) (configstore.AuthRecord, error)
	Close()
}

type Factory interface {
	Open(context.Context, configstore.AuthRecord) (Browser, error)
}
type Commit func(context.Context, configstore.AuthRecord) error
