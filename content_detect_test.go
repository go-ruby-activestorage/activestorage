package activestorage

import "testing"

// Golden results captured from Marcel::MimeType.for(bytes, name:) — the exact
// call ActiveStorage's extract_content_type makes.
func TestDetectContentTypeMagic(t *testing.T) {
	cases := []struct {
		name string
		data string
		file string
		want string
	}{
		{"png", "\x89PNG\r\n\x1a\n", "", "image/png"},
		{"jpeg", "\xff\xd8\xff\xe0\x00\x10JFIF", "", "image/jpeg"},
		{"gif87", "GIF87a", "", "image/gif"},
		{"gif89", "GIF89a", "", "image/gif"},
		{"pdf", "%PDF-1.4", "", "application/pdf"},
		{"gzip", "\x1f\x8b\x08", "", "application/gzip"},
		{"zip", "PK\x03\x04", "", "application/zip"},
		{"tiff_ii", "II*\x00", "", "image/tiff"},
		{"tiff_mm", "MM\x00*", "", "image/tiff"},
		{"mp3", "ID3\x03\x00", "", "audio/mpeg"},
		{"ogg", "OggS\x00", "", "application/ogg"},
		{"flac", "fLaC\x00", "", "audio/flac"},
		{"xz", "\xfd7zXZ\x00", "", "application/x-xz"},
		{"zstd", "\x28\xb5\x2f\xfd", "", "application/zstd"},
		{"7z", "7z\xbc\xaf\x27\x1c", "", "application/x-7z-compressed"},
		{"elf", "\x7fELF", "", "application/x-elf"},
		{"class", "\xca\xfe\xba\xbe", "", "application/java-vm"},
		{"psd", "8BPS", "", "image/vnd.adobe.photoshop"},
		{"ico", "\x00\x00\x01\x00", "", "image/vnd.microsoft.icon"},
		{"mp4", "\x00\x00\x00\x18ftypmp42", "", "video/mp4"},
		{"webp", "RIFF\x00\x00\x00\x00WEBP", "", "image/webp"},
		{"wav", "RIFF\x00\x00\x00\x00WAVE", "", "audio/x-wav"},
		{"avi", "RIFF\x00\x00\x00\x00AVI ", "", "video/x-msvideo"},
		{"xml", "<?xml version=\"1.0\"?>", "", "application/xml"},
		{"html_doctype", "<!DOCTYPE html><body>", "", "text/html"},
		{"html_tag", "<html><head>", "", "text/html"},
		// Fallbacks: no magic -> name; nothing -> octet-stream.
		{"plain_no_name", "hello world", "", "application/octet-stream"},
		{"plain_with_name", "hello world", "greeting.txt", "text/plain"},
		{"name_pdf_text_body", "just text", "report.pdf", "application/pdf"},
		{"empty_no_name", "", "", "application/octet-stream"},
		{"empty_with_name", "", "x.txt", "text/plain"},
		// Magic + more-specific name (descendant relationships Marcel applies).
		{"svg_named", "<?xml version=\"1.0\"?><svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", "a.svg", "image/svg+xml"},
		// RIFF container that is not one we recognize -> fall through to name.
		{"riff_unknown_named", "RIFF\x00\x00\x00\x00XXXX", "song.mp3", "audio/mpeg"},
		// Zip magic with a plain name stays zip (name is not a zip descendant).
		{"zip_named_txt", "PK\x03\x04", "a.txt", "application/zip"},
	}
	for _, c := range cases {
		if got := DetectContentType([]byte(c.data), c.file); got != c.want {
			t.Fatalf("%s: DetectContentType = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestExtensionType(t *testing.T) {
	cases := map[string]string{
		"note.txt": "text/plain", "d.csv": "text/csv", "p.html": "text/html",
		"p.htm": "text/html", "d.xml": "application/xml", "d.json": "application/json",
		"i.svg": "image/svg+xml", "d.pdf": "application/pdf", "i.png": "image/png",
		"i.jpg": "image/jpeg", "i.jpeg": "image/jpeg", "i.gif": "image/gif",
		"i.webp": "image/webp", "i.tif": "image/tiff", "i.tiff": "image/tiff",
		"a.zip": "application/zip", "a.gz": "application/gzip", "s.mp3": "audio/mpeg",
		"v.mp4": "video/mp4", "d.text": "text/plain",
	}
	for name, want := range cases {
		if got := ContentTypeForFilename(name); got != want {
			t.Fatalf("ContentTypeForFilename(%q) = %q, want %q", name, got, want)
		}
	}
	// No extension and unknown extension both fall back to octet-stream.
	if got := ContentTypeForFilename("README"); got != defaultContentType {
		t.Fatalf("noext = %q", got)
	}
	if got := ContentTypeForFilename("data.zzznope"); got != defaultContentType {
		t.Fatalf("unknown = %q", got)
	}
}

func TestDetectMagicShortData(t *testing.T) {
	// Data shorter than a signature must not panic and must report no match.
	if m := detectMagic([]byte("PK")); m != "" {
		t.Fatalf("short data magic = %q", m)
	}
	// RIFF header present but truncated before the container tag.
	if m := detectMagic([]byte("RIFF\x00\x00")); m != "" {
		t.Fatalf("short riff magic = %q", m)
	}
}
