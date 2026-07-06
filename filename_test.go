package activestorage

import "testing"

func TestFilenameParts(t *testing.T) {
	f := NewFilename("Report Final.PDF")
	if got := f.ExtensionWithDelimiter(); got != ".PDF" {
		t.Fatalf("ExtensionWithDelimiter = %q", got)
	}
	if got := f.Extension(); got != "PDF" {
		t.Fatalf("Extension = %q", got)
	}
	if got := f.Base(); got != "Report Final" {
		t.Fatalf("Base = %q", got)
	}
}

func TestFilenameNoExtension(t *testing.T) {
	f := NewFilename("LICENSE")
	if got := f.ExtensionWithDelimiter(); got != "" {
		t.Fatalf("ExtensionWithDelimiter = %q, want empty", got)
	}
	if got := f.Extension(); got != "" {
		t.Fatalf("Extension = %q, want empty", got)
	}
	if got := f.Base(); got != "LICENSE" {
		t.Fatalf("Base = %q", got)
	}
}

func TestFilenameSanitized(t *testing.T) {
	f := NewFilename("  a/b:c;d|e%f$g\th\ri\nj\\k  ")
	got := f.Sanitized()
	want := "a-b-c-d-e-f-g-h-i-j-k"
	if got != want {
		t.Fatalf("Sanitized = %q, want %q", got, want)
	}
	if f.String() != want {
		t.Fatalf("String = %q, want %q", f.String(), want)
	}
}
