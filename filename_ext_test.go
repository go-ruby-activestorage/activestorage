package activestorage

import "testing"

// Golden results from Ruby's File.extname / File.basename and
// ActiveStorage::Filename#sanitized.
func TestFilenameRubyExtnameSemantics(t *testing.T) {
	cases := []struct {
		raw, ext, base string
	}{
		{"racecar.jpg", ".jpg", "racecar"},
		{"racecar", "", "racecar"},
		{".gitignore", "", ".gitignore"},
		{"a.tar.gz", ".gz", "a.tar"},
		{"a.", ".", "a"},
		{"..foo", "", "..foo"},
		{"foo.bar.", ".", "foo.bar"},
		{".hidden.txt", ".txt", ".hidden"},
		{"LICENSE", "", "LICENSE"},
		{"dir/sub/file.png", ".png", "file"},
	}
	for _, c := range cases {
		f := NewFilename(c.raw)
		if got := f.ExtensionWithDelimiter(); got != c.ext {
			t.Fatalf("%q: ext = %q, want %q", c.raw, got, c.ext)
		}
		if got := f.Base(); got != c.base {
			t.Fatalf("%q: base = %q, want %q", c.raw, got, c.base)
		}
	}
}

func TestFilenameSanitizedFullCharset(t *testing.T) {
	// Matches Ruby: tr("\u{202E}%$|:;/<>?*\"\t\r\n\\", "-").
	f := NewFilename("foo<bar>baz?qux*.\"txt\"")
	if got := f.Sanitized(); got != "foo-bar-baz-qux-.-txt-" {
		t.Fatalf("sanitized = %q", got)
	}
	// The RIGHT-TO-LEFT OVERRIDE is stripped too.
	if got := NewFilename("a‮b").Sanitized(); got != "a-b" {
		t.Fatalf("rtl sanitized = %q", got)
	}
}
