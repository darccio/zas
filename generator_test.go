package zas

import "testing"

func TestNewGeneratorSetsVerboseAndFull(t *testing.T) {
	gen := NewGenerator(true, true)
	if !gen.Verbose {
		t.Error("NewGenerator(true, true).Verbose = false, want true")
	}
	if !gen.Full {
		t.Error("NewGenerator(true, true).Full = false, want true")
	}

	gen = NewGenerator(false, false)
	if gen.Verbose {
		t.Error("NewGenerator(false, false).Verbose = true, want false")
	}
	if gen.Full {
		t.Error("NewGenerator(false, false).Full = true, want false")
	}
}
