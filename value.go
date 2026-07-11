package activestorage

// Symbol marks a value that is a Ruby Symbol (e.g. a variation's :format value or
// :gravity argument), as distinct from a String. The distinction is invisible in
// JSON — both encode to a JSON string — but it changes the bytes of a
// Marshal.dump, and therefore a variation's digest, so it is preserved here.
type Symbol string

// Entry is one key/value pair of an ordered Ruby hash. The key is a Symbol name
// (Active Storage deep-symbolizes transformation keys and builds its
// message-verifier payloads with symbol keys). Value is one of: int, int64,
// string, Symbol, bool, nil, []any, or Hash.
type Entry struct {
	Key   string
	Value any
}

// Hash is an ordered Ruby hash with symbol keys. Order is significant: it is the
// insertion order Ruby preserves, which fixes the byte layout of both JSON.dump
// and Marshal.dump (and hence a signed key or a variation digest). It is used for
// variation transformations and for message-verifier payloads.
type Hash []Entry

// Get returns the value stored under key and whether it was present.
func (h Hash) Get(key string) (any, bool) {
	for _, e := range h {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}
