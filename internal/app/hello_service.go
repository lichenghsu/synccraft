// Package app holds the Wails services exposed to the frontend.
package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// CountTick is the payload of the count:tick event.
type CountTick struct {
	Current int `json:"current"`
	Total   int `json:"total"`
}

const EventCountTick = "count:tick"

func init() {
	application.RegisterEvent[CountTick](EventCountTick)
}

var ErrEmptyName = errors.New("name must not be empty")

// HelloService demonstrates calls, errors, cancellation and events.
type HelloService struct {
	emit func(name string, data any)
}

func NewHelloService() *HelloService {
	return &HelloService{
		emit: func(name string, data any) {
			if app := application.Get(); app != nil {
				app.Event.Emit(name, data)
			}
		},
	}
}

// Hello returns a greeting, or an error if name is blank.
func (s *HelloService) Hello(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrEmptyName
	}
	return "Hello, " + name + "!", nil
}

// Count counts from 1 to total, emitting count:tick on each step.
// It stops early when the frontend cancels the call.
func (s *HelloService) Count(ctx context.Context, total int, intervalMs int) (int, error) {
	if total <= 0 {
		return 0, errors.New("total must be positive")
	}
	if intervalMs <= 0 {
		intervalMs = 100
	}
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	for i := 1; i <= total; i++ {
		select {
		case <-ctx.Done():
			return i - 1, ctx.Err()
		case <-ticker.C:
			s.emit(EventCountTick, CountTick{Current: i, Total: total})
		}
	}
	return total, nil
}
