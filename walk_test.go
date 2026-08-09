package zas

import (
	"errors"
	"testing"
)

// B5: walk/reaper must return the incoming filepath.Walk error immediately,
// instead of touching a possibly-nil FileInfo first.

func TestWalkPropagatesErr(t *testing.T) {
	gen := &Generator{}
	wantErr := errors.New("permission denied")
	if err := gen.walk("noperm", nil, wantErr); !errors.Is(err, wantErr) {
		t.Fatalf("walk() error = %v, want %v", err, wantErr)
	}
}

func TestReaperPropagatesErr(t *testing.T) {
	gen := &Generator{}
	wantErr := errors.New("permission denied")
	if err := gen.reaper("noperm", nil, wantErr); !errors.Is(err, wantErr) {
		t.Fatalf("reaper() error = %v, want %v", err, wantErr)
	}
}
