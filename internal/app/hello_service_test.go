package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestService() (*HelloService, *[]CountTick) {
	var ticks []CountTick
	s := &HelloService{emit: func(_ string, data any) {
		ticks = append(ticks, data.(CountTick))
	}}
	return s, &ticks
}

func TestHello(t *testing.T) {
	s, _ := newTestService()

	got, err := s.Hello("  Miles ")
	if err != nil || got != "Hello, Miles!" {
		t.Fatalf("Hello() = %q, %v", got, err)
	}

	if _, err := s.Hello("   "); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("Hello(blank) error = %v, want ErrEmptyName", err)
	}
}

func TestCountCompletes(t *testing.T) {
	s, ticks := newTestService()

	n, err := s.Count(context.Background(), 3, 1)
	if err != nil || n != 3 {
		t.Fatalf("Count() = %d, %v", n, err)
	}
	if len(*ticks) != 3 || (*ticks)[2] != (CountTick{Current: 3, Total: 3}) {
		t.Fatalf("ticks = %+v", *ticks)
	}
}

func TestCountCancelled(t *testing.T) {
	s, _ := newTestService()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	n, err := s.Count(ctx, 1000, 10)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Count() error = %v, want deadline exceeded", err)
	}
	if n >= 1000 {
		t.Fatalf("Count() = %d, should have stopped early", n)
	}
}

func TestCountRejectsNonPositive(t *testing.T) {
	s, _ := newTestService()
	if _, err := s.Count(context.Background(), 0, 1); err == nil {
		t.Fatal("Count(0) should fail")
	}
}
