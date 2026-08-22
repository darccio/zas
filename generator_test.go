package zas

import "testing"

func TestNewGeneratorSetsVerboseAndFull(t *testing.T) {
	gen := NewGenerator(true, true, true)
	if !gen.Verbose {
		t.Error("NewGenerator(true, true, true).Verbose = false, want true")
	}
	if !gen.Full {
		t.Error("NewGenerator(true, true, true).Full = false, want true")
	}
	if !gen.NoPlugins {
		t.Error("NewGenerator(true, true, true).NoPlugins = false, want true")
	}

	gen = NewGenerator(false, false, false)
	if gen.Verbose {
		t.Error("NewGenerator(false, false, false).Verbose = true, want false")
	}
	if gen.Full {
		t.Error("NewGenerator(false, false, false).Full = true, want false")
	}
	if gen.NoPlugins {
		t.Error("NewGenerator(false, false, false).NoPlugins = true, want false")
	}
}
