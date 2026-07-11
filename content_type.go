package activestorage

import (
	"bytes"
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
	if t := extensionType(strings.ToLower(strings.TrimPrefix(ext, "."))); t != "" {
		return t
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

// magicRule matches a leading magic-byte signature. offset is where sig must
// appear; container (when set) is a second signature checked at containerAt,
// letting one RIFF wrapper resolve to several types.
type magicRule struct {
	offset      int
	sig         []byte
	containerAt int
	container   []byte
	mime        string
}

// magicTable is a faithful subset of Marcel's magic database — the signatures
// whose results were pinned against Marcel::MimeType.for. It is intentionally
// scoped (not the full freedesktop shared-mime database): unknown leading bytes
// fall through to name-based detection, exactly as Marcel falls back when no
// magic matches.
var magicTable = []magicRule{
	{sig: []byte("\x89PNG\r\n\x1a\n"), mime: "image/png"},
	{sig: []byte("\xff\xd8\xff"), mime: "image/jpeg"},
	{sig: []byte("GIF87a"), mime: "image/gif"},
	{sig: []byte("GIF89a"), mime: "image/gif"},
	{sig: []byte("%PDF"), mime: "application/pdf"},
	{sig: []byte("\x1f\x8b"), mime: "application/gzip"},
	{sig: []byte("PK\x03\x04"), mime: "application/zip"},
	{sig: []byte("PK\x05\x06"), mime: "application/zip"},
	{sig: []byte("PK\x07\x08"), mime: "application/zip"},
	{sig: []byte("II*\x00"), mime: "image/tiff"},
	{sig: []byte("MM\x00*"), mime: "image/tiff"},
	{sig: []byte("ID3"), mime: "audio/mpeg"},
	{sig: []byte("OggS"), mime: "application/ogg"},
	{sig: []byte("fLaC"), mime: "audio/flac"},
	{sig: []byte("\xfd7zXZ\x00"), mime: "application/x-xz"},
	{sig: []byte("\x28\xb5\x2f\xfd"), mime: "application/zstd"},
	{sig: []byte("7z\xbc\xaf\x27\x1c"), mime: "application/x-7z-compressed"},
	{sig: []byte("\x7fELF"), mime: "application/x-elf"},
	{sig: []byte("\xca\xfe\xba\xbe"), mime: "application/java-vm"},
	{sig: []byte("8BPS"), mime: "image/vnd.adobe.photoshop"},
	{sig: []byte("\x00\x00\x01\x00"), mime: "image/vnd.microsoft.icon"},
	{offset: 4, sig: []byte("ftyp"), mime: "video/mp4"},
	{sig: []byte("RIFF"), containerAt: 8, container: []byte("WEBP"), mime: "image/webp"},
	{sig: []byte("RIFF"), containerAt: 8, container: []byte("WAVE"), mime: "audio/x-wav"},
	{sig: []byte("RIFF"), containerAt: 8, container: []byte("AVI "), mime: "video/x-msvideo"},
	{sig: []byte("<?xml"), mime: "application/xml"},
	{sig: []byte("<!DOCTYPE html"), mime: "text/html"},
	{sig: []byte("<!doctype html"), mime: "text/html"},
	{sig: []byte("<html"), mime: "text/html"},
	{sig: []byte("<HTML"), mime: "text/html"},
}

// magicParents records the "is a more specific kind of" relationships Marcel uses
// to prefer a name-derived type over a magic-derived one: e.g. bytes read as
// generic application/xml but named ".svg" resolve to image/svg+xml, and a Zip
// container named ".docx" resolves to the Office type.
var magicParents = map[string][]string{
	"application/xml": {"image/svg+xml"},
	"application/zip": {
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/epub+zip",
		"application/vnd.oasis.opendocument.text",
		"application/java-archive",
	},
}

// detectMagic returns the MIME type implied by data's leading bytes, or "" when
// no signature matches.
func detectMagic(data []byte) string {
	for _, r := range magicTable {
		end := r.offset + len(r.sig)
		if len(data) < end || !bytes.Equal(data[r.offset:end], r.sig) {
			continue
		}
		if r.container != nil {
			cend := r.containerAt + len(r.container)
			if len(data) < cend || !bytes.Equal(data[r.containerAt:cend], r.container) {
				continue
			}
		}
		return r.mime
	}
	return ""
}

// DetectContentType reproduces ActiveStorage's extract_content_type, which is
// Marcel::MimeType.for(io, name:) for the common cases: it prefers the type
// implied by the leading magic bytes, upgrades a generic magic type to a more
// specific one implied by the name (xml→svg, zip→docx, …), and, when no magic
// matches, falls back to the name's extension — returning application/octet-stream
// only when neither yields anything.
func DetectContentType(data []byte, name string) string {
	magic := detectMagic(data)
	nameType := ContentTypeForFilename(name)
	if magic == "" {
		return nameType
	}
	if children, ok := magicParents[magic]; ok && nameType != defaultContentType {
		for _, c := range children {
			if c == nameType {
				return nameType
			}
		}
	}
	return magic
}

// extensionType maps a lowercase extension to the MIME type Marcel/Active Storage
// reports for it, for the extensions whose mapping the host mime database does not
// reliably provide. It keeps name-based detection deterministic across platforms.
func extensionType(ext string) string {
	switch ext {
	case "txt", "text":
		return "text/plain"
	case "csv":
		return "text/csv"
	case "html", "htm":
		return "text/html"
	case "xml":
		return "application/xml"
	case "json":
		return "application/json"
	case "svg":
		return "image/svg+xml"
	case "pdf":
		return "application/pdf"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "tif", "tiff":
		return "image/tiff"
	case "zip":
		return "application/zip"
	case "gz":
		return "application/gzip"
	case "mp3":
		return "audio/mpeg"
	case "mp4":
		return "video/mp4"
	}
	return ""
}
