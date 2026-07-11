package activestorage

import "testing"

func TestRubyJSONScalars(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{nil, "null"},
		{true, "true"},
		{false, "false"},
		{int(42), "42"},
		{int64(-7), "-7"},
		{"hi", `"hi"`},
		{Symbol("jpg"), `"jpg"`},
		{[]any{1, "a", true}, `[1,"a",true]`},
		{Hash{{Key: "a", Value: 1}, {Key: "b", Value: []any{2, 3}}}, `{"a":1,"b":[2,3]}`},
		// Ruby json leaves /, <, >, & unescaped (Go's encoder would escape some).
		{"a/b<c>&d", `"a/b<c>&d"`},
		// Named control-character short forms match the json gem.
		{"tab\tnl\nret\rq\"bs\\\bff\f", `"tab\tnl\nret\rq\"bs\\\bff\f"`},
	}
	for _, c := range cases {
		got, err := rubyJSON(c.v)
		if err != nil {
			t.Fatalf("rubyJSON(%v) error %v", c.v, err)
		}
		if string(got) != c.want {
			t.Fatalf("rubyJSON(%v) = %s, want %s", c.v, got, c.want)
		}
	}
	// A C0 control below the named set escapes as \u00xx, like the json gem.
	got, _ := rubyJSON("\x01")
	if want := "\"\\u0001\""; string(got) != want {
		t.Fatalf("control escape = %s, want %s", got, want)
	}
}

type notEncodable struct{}

func TestRubyJSONUnsupported(t *testing.T) {
	if _, err := rubyJSON(notEncodable{}); err == nil {
		t.Fatal("expected error for unsupported type")
	}
	// Error propagation from within an array and a hash value.
	if _, err := rubyJSON([]any{notEncodable{}}); err == nil {
		t.Fatal("expected array element error")
	}
	if _, err := rubyJSON(Hash{{Key: "x", Value: notEncodable{}}}); err == nil {
		t.Fatal("expected hash value error")
	}
}
