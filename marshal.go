package activestorage

import (
	"bytes"
	"fmt"
)

// rubyMarshal reproduces Ruby's Marshal.dump for the value types that appear in
// an Active Storage variation's transformations hash. Active Storage derives a
// variation's digest as OpenSSL::Digest::SHA1.base64digest(Marshal.dump(transformations)),
// so the bytes here must match MRI's Marshal exactly — including the version
// prefix, the small-integer packing, and the symbol table with back-references.
//
// Supported values: nil, bool, int/int64, string (UTF-8, encoded as MRI does with
// an :E => true instance variable), Symbol, []any, and Hash. Anything else is an
// error rather than a silently wrong digest.
type marshaler struct {
	buf  bytes.Buffer
	syms map[string]int // symbol -> table index, for symlink back-references
}

func rubyMarshal(v any) ([]byte, error) {
	m := &marshaler{syms: map[string]int{}}
	m.buf.WriteByte(4) // major version
	m.buf.WriteByte(8) // minor version
	if err := m.encode(v); err != nil {
		return nil, err
	}
	return m.buf.Bytes(), nil
}

func (m *marshaler) encode(v any) error {
	switch x := v.(type) {
	case nil:
		m.buf.WriteByte('0')
	case bool:
		if x {
			m.buf.WriteByte('T')
		} else {
			m.buf.WriteByte('F')
		}
	case int:
		m.buf.WriteByte('i')
		m.long(x)
	case int64:
		m.buf.WriteByte('i')
		m.long(int(x))
	case Symbol:
		m.symbol(string(x))
	case string:
		m.str(x)
	case []any:
		m.buf.WriteByte('[')
		m.long(len(x))
		for _, e := range x {
			if err := m.encode(e); err != nil {
				return err
			}
		}
	case Hash:
		m.buf.WriteByte('{')
		m.long(len(x))
		for _, e := range x {
			m.symbol(e.Key)
			if err := m.encode(e.Value); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("activestorage: cannot Marshal %T", v)
	}
	return nil
}

// symbol writes a Symbol, using a symlink (";" + index) when the symbol has
// already been emitted, exactly as MRI's Marshal deduplicates symbols.
func (m *marshaler) symbol(s string) {
	if idx, ok := m.syms[s]; ok {
		m.buf.WriteByte(';')
		m.long(idx)
		return
	}
	m.syms[s] = len(m.syms)
	m.buf.WriteByte(':')
	m.long(len(s))
	m.buf.WriteString(s)
}

// str writes a String as MRI does for a UTF-8 string: an instance-variable
// wrapper (I) around the raw string ('"') carrying a single :E => true, the
// encoding flag marking UTF-8.
func (m *marshaler) str(s string) {
	m.buf.WriteByte('I')
	m.buf.WriteByte('"')
	m.long(len(s))
	m.buf.WriteString(s)
	m.long(1) // one instance variable
	m.symbol("E")
	m.buf.WriteByte('T')
}

// long writes an integer in Marshal's variable-length packing: 0 as a zero byte,
// small magnitudes (|n| < 123) as a single biased byte, and larger values as a
// length-prefixed little-endian sequence.
func (m *marshaler) long(n int) {
	switch {
	case n == 0:
		m.buf.WriteByte(0)
	case n > 0 && n < 123:
		m.buf.WriteByte(byte(n + 5))
	case n < 0 && n > -124:
		m.buf.WriteByte(byte(n - 5))
	case n > 0:
		var b []byte
		for v := n; v > 0; v >>= 8 {
			b = append(b, byte(v&0xff))
		}
		m.buf.WriteByte(byte(len(b)))
		m.buf.Write(b)
	default: // n <= -124
		var b []byte
		for v := n; v < -1; v >>= 8 {
			b = append(b, byte(v&0xff))
		}
		m.buf.WriteByte(byte(256 - len(b)))
		m.buf.Write(b)
	}
}
