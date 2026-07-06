package activestorage

import (
	"path/filepath"
	"strings"
)

// filenameSanitizer replaces the characters ActiveStorage::Filename#sanitized
// strips out — the Unicode RIGHT-TO-LEFT OVERRIDE plus the shell/path-hostile set
// "%$|:;/", tab, CR, LF and backslash — each with a single "-".
var filenameSanitizer = strings.NewReplacer(
	"‮", "-",
	"%", "-",
	"$", "-",
	"|", "-",
	":", "-",
	";", "-",
	"/", "-",
	"\t", "-",
	"\r", "-",
	"\n", "-",
	"\\", "-",
)

// Filename is a value type mirroring ActiveStorage::Filename: it exposes a file's
// base name, extension, and a sanitized form safe to persist or serve.
type Filename struct {
	raw string
}

// NewFilename wraps a raw filename string.
func NewFilename(name string) Filename { return Filename{raw: name} }

// ExtensionWithDelimiter returns the extension including its leading dot
// (e.g. ".pdf"), or "" when there is none.
func (f Filename) ExtensionWithDelimiter() string { return filepath.Ext(f.raw) }

// Extension returns the extension without its leading dot (e.g. "pdf"), or "".
func (f Filename) Extension() string {
	return strings.TrimPrefix(f.ExtensionWithDelimiter(), ".")
}

// Base returns the filename with its extension removed.
func (f Filename) Base() string {
	return strings.TrimSuffix(f.raw, f.ExtensionWithDelimiter())
}

// Sanitized returns the filename with surrounding whitespace trimmed and unsafe
// characters replaced by "-".
func (f Filename) Sanitized() string {
	return filenameSanitizer.Replace(strings.TrimSpace(f.raw))
}

// String returns the sanitized filename, matching ActiveStorage::Filename#to_s.
func (f Filename) String() string { return f.Sanitized() }
