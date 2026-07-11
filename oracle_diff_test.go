package activestorage

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// rubyOracle runs script with Ruby and returns its stdout, skipping the test when
// Ruby or the activestorage/activesupport/marcel gems are unavailable — the
// differential oracle is optional (it is not present in the pure-Go CI lanes).
func rubyOracle(t *testing.T, script string) string {
	t.Helper()
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not installed; skipping gem differential oracle")
	}
	probe := exec.Command("ruby", "-e", `require "active_support/all"; require "marcel"; require "openssl"`)
	if out, err := probe.CombinedOutput(); err != nil {
		t.Skipf("activesupport/marcel gems unavailable: %s", out)
	}
	out, err := exec.Command("ruby", "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby oracle failed: %v\n%s", err, out)
	}
	return string(out)
}

// The secret matches oracleSecret() ("0"*64) and the verifier config matches
// RailsVerifier (JSON serializer, SHA-1 HMAC, url-safe).
const rubyPreamble = `
require "active_support/all"
require "marcel"
require "openssl"
secret = "0"*64
V = ActiveSupport::MessageVerifier.new(secret, serializer: JSON, url_safe: true)
`

func TestDifferentialSignedIDAndVariation(t *testing.T) {
	script := rubyPreamble + `
puts "SID=" + V.generate(1234, purpose: "blob_id")
tr = {resize_to_limit: [640, 480], colourspace: "b-w", format: :webp, sharpen: true}
puts "DIGEST=" + OpenSSL::Digest::SHA1.base64digest(Marshal.dump(tr))
puts "VKEY=" + V.generate(tr, purpose: "variation")
`
	got := parseKV(rubyOracle(t, script))

	v := NewRailsVerifier(oracleSecret())
	sid, _ := v.Generate(int64(1234), "blob_id", time.Time{})
	if sid != got["SID"] {
		t.Fatalf("signed_id mismatch:\n go   %s\n ruby %s", sid, got["SID"])
	}

	tr := Hash{
		{Key: "resize_to_limit", Value: []any{640, 480}},
		{Key: "colourspace", Value: "b-w"},
		{Key: "format", Value: Symbol("webp")},
		{Key: "sharpen", Value: true},
	}
	va := NewVariation(v, tr)
	if dg, _ := va.Digest(); dg != got["DIGEST"] {
		t.Fatalf("variation digest mismatch:\n go   %s\n ruby %s", dg, got["DIGEST"])
	}
	if vk, _ := va.Key(); vk != got["VKEY"] {
		t.Fatalf("variation key mismatch:\n go   %s\n ruby %s", vk, got["VKEY"])
	}
}

func TestDifferentialContentType(t *testing.T) {
	// Byte samples (as escaped Ruby strings) paired with the Go bytes and name.
	script := rubyPreamble + `
def ct(bytes, name); Marcel::MimeType.for(bytes.b, name: name); end
puts "PNG="  + ct("\x89PNG\r\n\x1a\n", nil)
puts "PDF="  + ct("%PDF-1.5", nil)
puts "GZIP=" + ct("\x1f\x8b\x08\x00", nil)
puts "TXT="  + ct("plain body", "notes.txt")
puts "NONE=" + ct("plain body", nil)
puts "SVG="  + ct("<?xml version=\"1.0\"?><svg xmlns=\"http://www.w3.org/2000/svg\"/>", "logo.svg")
`
	got := parseKV(rubyOracle(t, script))
	cases := map[string]struct {
		data, name string
	}{
		"PNG":  {"\x89PNG\r\n\x1a\n", ""},
		"PDF":  {"%PDF-1.5", ""},
		"GZIP": {"\x1f\x8b\x08\x00", ""},
		"TXT":  {"plain body", "notes.txt"},
		"NONE": {"plain body", ""},
		"SVG":  {"<?xml version=\"1.0\"?><svg xmlns=\"http://www.w3.org/2000/svg\"/>", "logo.svg"},
	}
	for k, c := range cases {
		if g := DetectContentType([]byte(c.data), c.name); g != got[k] {
			t.Fatalf("%s content type mismatch:\n go   %s\n ruby %s", k, g, got[k])
		}
	}
}

func TestDifferentialChecksumAndFilename(t *testing.T) {
	script := rubyPreamble + `
require "digest"
puts "CK=" + Digest::MD5.base64digest("the quick brown fox")
puts "SAN=" + ActiveStorage::Filename.new("a/b:c<d>e?f*g\"h.TXT").sanitized rescue puts("SAN=SKIP")
puts "EXT=" + ActiveStorage::Filename.new(".config.yml").extension rescue puts("EXT=SKIP")
`
	got := parseKV(rubyOracle(t, script))
	if ck := checksumOf([]byte("the quick brown fox")); ck != got["CK"] {
		t.Fatalf("checksum mismatch: go %s ruby %s", ck, got["CK"])
	}
	if got["SAN"] != "SKIP" {
		if s := NewFilename("a/b:c<d>e?f*g\"h.TXT").Sanitized(); s != got["SAN"] {
			t.Fatalf("sanitized mismatch: go %q ruby %q", s, got["SAN"])
		}
	}
	if got["EXT"] != "SKIP" {
		if e := NewFilename(".config.yml").Extension(); e != got["EXT"] {
			t.Fatalf("extension mismatch: go %q ruby %q", e, got["EXT"])
		}
	}
}

func parseKV(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if i := strings.IndexByte(line, '='); i >= 0 {
			m[line[:i]] = strings.TrimRight(line[i+1:], "\r")
		}
	}
	return m
}
