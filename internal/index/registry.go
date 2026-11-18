package index

import (
	"lamina/internal/catalog"
	"lamina/internal/versions"
)

type Registry struct{ Trees map[string]*Tree }

// Build reconstructs index membership from retained versions by replaying each
// chain in sequence order with the same eligibility transition rules used
// online. A partial index therefore yields the same entries after restart as it
// did while commits were being applied, and eligibility intervals remain
// anchored to the versions that produced them.
func Build(c *catalog.Catalog, s *versions.Store) *Registry {
	r := &Registry{Trees: map[string]*Tree{}}
	for name, definition := range c.Indexes {
		tree := New()
		for key, chain := range s.Tables[definition.Relation] {
			for _, v := range chain {
				if v.Deleted || !catalog.HoldsAll(v.Row, definition.Where) {
					tree.Close(key, v.Sequence)
					continue
				}
				tree.Update(v.Row[definition.Column], key, v.Sequence)
			}
		}
		r.Trees[name] = tree
	}
	return r
}

// Apply updates membership for one committed record. Deletes and writes that
// stop satisfying a condition close the previous association at seq; a write
// that satisfies the condition opens a new association (or extends an identical
// value), so candidate eligibility changes together with the commit.
func (r *Registry) Apply(c *catalog.Catalog, writes []versions.Mutation, seq uint64) {
	for _, m := range writes {
		for name, definition := range c.Indexes {
			if definition.Relation != m.Relation {
				continue
			}
			tree, ok := r.Trees[name]
			if !ok {
				continue
			}
			if m.Deleted || !catalog.HoldsAll(m.Row, definition.Where) {
				tree.Close(m.Key, seq)
				continue
			}
			tree.Update(m.Row[definition.Column], m.Key, seq)
		}
	}
}
