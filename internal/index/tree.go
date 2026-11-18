package index

import (
	"lamina/internal/datum"
	"sort"
)

// Entry is one historical candidate association. A partial index only records
// associations while the committed row satisfied the index condition, so the
// half-open sequence interval [From, Until) marks exactly when the row was a
// legitimate candidate. Until == 0 means the association is still open.
type Entry struct {
	Value datum.Value
	Key   string
	From  uint64
	Until uint64
}

type Tree struct {
	entries []*Entry
	byKey   map[string][]*Entry
}

func New() *Tree { return &Tree{byKey: map[string][]*Entry{}} }

func (t *Tree) addInterval(value datum.Value, key string, from, until uint64) {
	if value.Null() || until != 0 && until <= from {
		return
	}
	e := &Entry{Value: value, Key: key, From: from, Until: until}
	at := sort.Search(len(t.entries), func(i int) bool {
		n := datum.Order(t.entries[i].Value, value)
		if n != 0 {
			return n > 0
		}
		if t.entries[i].Key != key {
			return t.entries[i].Key > key
		}
		if t.entries[i].From != from {
			return t.entries[i].From > from
		}
		return t.entries[i].Until > until
	})
	t.entries = append(t.entries, nil)
	copy(t.entries[at+1:], t.entries[at:])
	t.entries[at] = e
	t.byKey[key] = append(t.byKey[key], e)
}

// Update applies a committed row for key at seq. If the row is still a
// candidate with the same value as its current open association the interval is
// extended; otherwise the old association closes at seq and a new one opens.
// This keeps both current reads and older snapshots correct.
func (t *Tree) Update(value datum.Value, key string, seq uint64) {
	if value.Null() {
		t.Close(key, seq)
		return
	}
	for _, e := range t.byKey[key] {
		if e.Until != 0 {
			continue
		}
		if datum.Order(e.Value, value) == 0 {
			return
		}
		e.Until = seq
	}
	t.AddAt(value, key, seq)
}

// AddAt records that key satisfied the index condition with value at sequence
// seq, without inspecting prior open associations.
func (t *Tree) AddAt(value datum.Value, key string, seq uint64) {
	t.addInterval(value, key, seq, 0)
}

// Close ends every open association for key at seq. It is used when a row is
// deleted or stops satisfying the index condition.
func (t *Tree) Close(key string, seq uint64) {
	for _, e := range t.byKey[key] {
		if e.Until == 0 {
			e.Until = seq
		}
	}
}

func (e *Entry) covers(at uint64) bool {
	return e.From <= at && (e.Until == 0 || at < e.Until)
}

// Candidates returns the keys that were associated with a value matching the
// comparison at the requested snapshot. Entries whose interval does not cover
// the snapshot are historical noise for that read and must be skipped.
func (t *Tree) Candidates(op string, value datum.Value, at uint64) []string {
	if value.Null() {
		return nil
	}
	lo := sort.Search(len(t.entries), func(i int) bool { return datum.Order(t.entries[i].Value, value) >= 0 })
	hi := sort.Search(len(t.entries), func(i int) bool { return datum.Order(t.entries[i].Value, value) > 0 })
	begin, end := 0, len(t.entries)
	switch op {
	case "=":
		begin, end = lo, hi
	case ">":
		begin = hi
	case ">=":
		begin = lo
	case "<":
		end = lo
	case "<=":
		end = hi
	default:
		return nil
	}
	out := make([]string, 0, end-begin)
	seen := map[string]bool{}
	for _, e := range t.entries[begin:end] {
		if !e.covers(at) {
			continue
		}
		if !seen[e.Key] {
			seen[e.Key] = true
			out = append(out, e.Key)
		}
	}
	return out
}

func (t *Tree) Size() int { return len(t.entries) }
