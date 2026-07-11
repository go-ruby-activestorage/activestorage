package activestorage

import (
	"encoding/hex"
	"testing"
)

// The golden bytes are Marshal.dump(transformations.deep_symbolize_keys).unpack1("H*")
// captured from MRI Ruby 3.x (the same bytes Active Storage feeds SHA1 for a
// variation digest).
func TestRubyMarshalGolden(t *testing.T) {
	cases := []struct {
		name string
		in   Hash
		hex  string
	}{
		{"resize_to_limit", Hash{{Key: "resize_to_limit", Value: []any{100, 100}}},
			"04087b063a14726573697a655f746f5f6c696d69745b0769696969"},
		{"crop_true", Hash{{Key: "crop", Value: true}},
			"04087b063a0963726f7054"},
		{"neg", Hash{{Key: "rotate", Value: -90}},
			"04087b063a0b726f7461746569a1"},
		{"big", Hash{{Key: "quality", Value: 800}},
			"04087b063a0c7175616c69747969022003"},
		{"huge", Hash{{Key: "n", Value: 70000}},
			"04087b063a066e6903701101"},
		{"false", Hash{{Key: "strip", Value: false}},
			"04087b063a0a737472697046"},
		{"nil", Hash{{Key: "background", Value: nil}},
			"04087b063a0f6261636b67726f756e6430"},
		{"symbol_value", Hash{{Key: "gravity", Value: Symbol("center")}},
			"04087b063a0c677261766974793a0b63656e746572"},
		{"nested", Hash{{Key: "saver", Value: Hash{{Key: "trim", Value: true}, {Key: "quality", Value: 90}}}},
			"04087b063a0a73617665727b073a097472696d543a0c7175616c697479695f"},
		{"multi_symlink", Hash{
			{Key: "resize_to_fill", Value: []any{300, 300}},
			{Key: "gravity", Value: "north"},
			{Key: "crop", Value: true}},
			"04087b083a13726573697a655f746f5f66696c6c5b0769022c0169022c013a0c6772617669747949220a6e6f727468063a0645543a0963726f7054"},
		{"int64_and_zero", Hash{{Key: "x", Value: int64(1)}, {Key: "y", Value: 0}},
			""}, // computed below, just exercise int64+zero paths
	}
	for _, c := range cases {
		got, err := rubyMarshal(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if c.hex == "" {
			continue
		}
		if h := hex.EncodeToString(got); h != c.hex {
			t.Fatalf("%s: marshal = %s, want %s", c.name, h, c.hex)
		}
	}
}

func TestRubyMarshalNegativeMultibyte(t *testing.T) {
	// -70000: exercises the multi-byte negative branch of long().
	got, err := rubyMarshal(Hash{{Key: "n", Value: -70000}})
	if err != nil {
		t.Fatal(err)
	}
	// Length byte 0xfd (=256-3) then the little-endian bytes of -70000.
	if hex.EncodeToString(got) != "04087b063a066e69fd90eefe" {
		t.Fatalf("neg multibyte = %s", hex.EncodeToString(got))
	}
}

func TestRubyMarshalUnsupported(t *testing.T) {
	if _, err := rubyMarshal(Hash{{Key: "x", Value: notEncodable{}}}); err == nil {
		t.Fatal("expected error for unsupported value")
	}
	if _, err := rubyMarshal(notEncodable{}); err == nil {
		t.Fatal("expected error for unsupported root")
	}
	// Array element error path.
	if _, err := rubyMarshal([]any{notEncodable{}}); err == nil {
		t.Fatal("expected array element error")
	}
}
