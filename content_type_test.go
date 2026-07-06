package activestorage

import (
	"mime"
	"testing"
)

func TestContentTypeForFilename(t *testing.T) {
	// Register a deterministic mapping so the assertions do not depend on the
	// host's system MIME database.
	_ = mime.AddExtensionType(".png", "image/png")
	_ = mime.AddExtensionType(".css", "text/css; charset=utf-8")

	if got := ContentTypeForFilename("photo.png"); got != "image/png" {
		t.Fatalf("png = %q, want image/png", got)
	}
	// A type carrying parameters must be trimmed to the bare type.
	if got := ContentTypeForFilename("style.css"); got != "text/css" {
		t.Fatalf("css = %q, want text/css", got)
	}
	// No extension -> default.
	if got := ContentTypeForFilename("README"); got != defaultContentType {
		t.Fatalf("noext = %q, want %q", got, defaultContentType)
	}
	// Unknown extension -> default.
	if got := ContentTypeForFilename("data.zzznotatype"); got != defaultContentType {
		t.Fatalf("unknown = %q, want %q", got, defaultContentType)
	}
}
