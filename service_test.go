package activestorage

import (
	"errors"
	"testing"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	if _, err := reg.Default(); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("Default on empty = %v, want ErrServiceNotFound", err)
	}
	a := NewDiskService("a", t.TempDir())
	b := NewDiskService("b", t.TempDir())
	reg.Register(a).Register(b)

	// First registered becomes the default.
	def, err := reg.Default()
	if err != nil || def.Name() != "a" {
		t.Fatalf("Default = %v, %v", def, err)
	}
	got, err := reg.Fetch("b")
	if err != nil || got != b {
		t.Fatalf("Fetch(b) = %v, %v", got, err)
	}
	if _, err := reg.Fetch("missing"); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("Fetch(missing) = %v", err)
	}
	reg.SetDefault("b")
	def, _ = reg.Default()
	if def.Name() != "b" {
		t.Fatalf("Default after SetDefault = %v", def.Name())
	}
}
