package activestorage

import (
	"bytes"
	"fmt"
	"strconv"
)

// rubyJSON serializes v the way Ruby's JSON.generate / JSON.dump does, which is
// what ActiveSupport::MessageVerifier's default (JSON) serializer emits. It is
// deliberately byte-compatible with the Ruby json gem rather than with Go's
// encoding/json, whose defaults differ (encoding/json HTML-escapes <, >, and &,
// and orders map keys lexicographically). Supported values mirror Hash's
// contract: nil, bool, int, int64, string, Symbol, []any, and Hash.
func rubyJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := rjEncode(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func rjEncode(b *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case string:
		rjString(b, x)
	case Symbol:
		rjString(b, string(x))
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := rjEncode(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case Hash:
		b.WriteByte('{')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			rjString(b, e.Key)
			b.WriteByte(':')
			if err := rjEncode(b, e.Value); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("activestorage: cannot JSON-encode %T", v)
	}
	return nil
}

// rjString writes s as a JSON string using the Ruby json gem's escaping rules:
// escape only ", \, and the C0 control characters (with \b \t \n \f \r short
// forms and \u00xx otherwise). Unlike Go's encoder it leaves /, <, >, & and every
// printable multibyte rune untouched, emitting raw UTF-8.
func rjString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
