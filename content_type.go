package activestorage

import (
	"mime"
	"path/filepath"
	"strings"
)

// defaultContentType is the fallback MIME type, matching Rails'
// ActiveStorage.binary_content_type / the octet-stream default.
const defaultContentType = "application/octet-stream"

// ContentTypeForFilename infers a MIME type from a filename's extension, dropping
// any parameters (e.g. "; charset=utf-8") to return a bare "type/subtype". When
// the extension is missing or unknown it returns application/octet-stream, the
// same conservative default Rails applies when content-type detection fails.
func ContentTypeForFilename(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return defaultContentType
	}
	t := mime.TypeByExtension(ext)
	if t == "" {
		return defaultContentType
	}
	if i := strings.IndexByte(t, ';'); i >= 0 {
		t = t[:i]
	}
	return strings.TrimSpace(t)
}
