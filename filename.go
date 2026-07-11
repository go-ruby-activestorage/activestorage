package activestorage

import (
	"strings"
)

// filenameSanitizer replaces exactly the characters ActiveStorage::Filename#sanitized
// strips — the Unicode RIGHT-TO-LEFT OVERRIDE (U+202E) plus the shell/path/URL-hostile
// set "%$|:;/<>?*\"" together with tab, CR, LF and backslash — each with a single "-".
// The character set matches the gem's tr("\u{202E}%$|:;/<>?*\"\t\r\n\\", "-") exactly.
var filenameSanitizer = strings.NewReplacer(
	"‮", "-",
	"%", "-",
	"$", "-",
	"|", "-",
	":", "-",
	";", "-",
	"/", "-",
	"<", "-",
	">", "-",
	"?", "-",
	"*", "-",
	"\"", "-",
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

// rubyExtname reproduces Ruby's File.extname: the extension is the substring from
// the final "." of the basename to the end, but only when that dot is preceded by
// at least one non-dot character. So "racecar.jpg" => ".jpg", "a.tar.gz" => ".gz",
// "a." => ".", but ".gitignore" => "" and "..foo" => "" (leading dots are not an
// extension). This differs from Go's filepath.Ext, which treats ".gitignore" as
// having extension ".gitignore".
func rubyExtname(name string) string {
	base := name
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	dot := strings.LastIndexByte(base, '.')
	if dot <= 0 {
		return ""
	}
	for i := 0; i < dot; i++ {
		if base[i] != '.' {
			return base[dot:]
		}
	}
	return ""
}

// basename returns the filename with any leading directory removed, mirroring the
// File.basename Active Storage applies before trimming the extension.
func (f Filename) basename() string {
	base := f.raw
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	return base
}

// ExtensionWithDelimiter returns the extension including its leading dot
// (e.g. ".pdf"), or "" when there is none.
func (f Filename) ExtensionWithDelimiter() string { return rubyExtname(f.raw) }

// Extension returns the extension without its leading dot (e.g. "pdf"), or "".
func (f Filename) Extension() string {
	return strings.TrimPrefix(f.ExtensionWithDelimiter(), ".")
}

// Base returns the basename with its extension removed
// (ActiveStorage::Filename#base).
func (f Filename) Base() string {
	return strings.TrimSuffix(f.basename(), f.ExtensionWithDelimiter())
}

// Sanitized returns the filename with surrounding whitespace trimmed and unsafe
// characters replaced by "-".
func (f Filename) Sanitized() string {
	return filenameSanitizer.Replace(strings.TrimSpace(f.raw))
}

// String returns the sanitized filename, matching ActiveStorage::Filename#to_s.
func (f Filename) String() string { return f.Sanitized() }
