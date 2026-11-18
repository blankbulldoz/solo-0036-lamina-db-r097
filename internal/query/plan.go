package query

import (
	"strconv"

	"lamina/internal/catalog"
	"lamina/internal/index"
	"sort"
)

// Consideration records why one index was or was not eligible for a query.
// It is execution fact, not a cost estimate.
type Consideration struct {
	Index     string `json:"index"`
	Eligible  bool   `json:"eligible"`
	Chosen    bool   `json:"chosen"`
	Reason    string `json:"reason"`
	Condition string `json:"condition,omitempty"`
}

// Choose selects an access path. An index is usable only when the planner can
// prove that every row matching the full WHERE conjunction also satisfies the
// index condition. Otherwise a partial index could omit legitimate rows and a
// scan must be used. The index never supplies final rows: every candidate is
// re-read from the requested snapshot and all predicates are rechecked.
func Choose(c *catalog.Catalog, registry *index.Registry, q Request, at uint64, total int) (Plan, []string) {
	plan := Plan{Access: "scan", Candidates: total, Snapshot: at, Residuals: len(q.Where), Reason: "no applicable index; full scan"}
	if q.ForceScan {
		plan.Reason = "force_scan requested"
		return plan, nil
	}
	names := make([]string, 0, len(c.Indexes))
	for name := range c.Indexes {
		names = append(names, name)
	}
	sort.Strings(names)
	var chosen []string
	bestCandidates := -1
	for _, name := range names {
		definition := c.Indexes[name]
		if definition.Relation != q.Relation {
			continue
		}
		condition := catalog.Describe(definition.Where)
		tree := registry.Trees[name]
		note := func(eligible, chosen bool, reason string) {
			plan.Considered = append(plan.Considered, Consideration{Index: name, Eligible: eligible, Chosen: chosen, Reason: reason, Condition: condition})
		}
		switch {
		case tree == nil:
			note(false, false, "index state unavailable")
			continue
		case definition.Created > at:
			note(false, false, "index did not exist at snapshot "+itoa(at))
			continue
		case !impliedByQuery(q.Where, definition.Where):
			note(false, false, "query does not prove condition ("+condition+"); using it could omit matching rows")
			continue
		}
		candidates, predicate, ok := bestRange(tree, definition.Column, q.Where, at)
		if !ok {
			note(false, false, "no range predicate on indexed column "+definition.Column)
			continue
		}
		reason := "condition (" + condition + ") is implied by predicate " + predicate.String() + "; " + strconv.Itoa(len(candidates)) + " candidates rechecked at snapshot " + itoa(at)
		improves := len(candidates) < plan.Candidates || plan.Access == "scan" && len(candidates) == 0
		if plan.Access == "scan" && bestCandidates >= 0 || plan.Access == "index" && !improves {
			note(true, false, reason)
			continue
		}
		if plan.Access == "index" && plan.Index != "" {
			for i := range plan.Considered {
				if plan.Considered[i].Index == plan.Index {
					plan.Considered[i].Chosen = false
				}
			}
		}
		bestCandidates = len(candidates)
		plan.Access = "index"
		plan.Index = name
		plan.Candidates = len(candidates)
		plan.Condition = definition.Where
		plan.Reason = reason
		chosen = candidates
		note(true, true, reason)
	}
	if plan.Access == "scan" {
		if len(plan.Considered) > 0 {
			plan.Reason = "no eligible index improved on the scan; full scan"
		}
		plan.Candidates = total
	}
	return plan, chosen
}

// bestRange returns the smallest candidate set among query range predicates on
// the indexed column. Only contiguous comparisons can drive an index range;
// null checks and != would need a full candidate walk and are therefore not
// allowed to select the index here.
func bestRange(tree *index.Tree, column string, where []catalog.Predicate, at uint64) ([]string, catalog.Predicate, bool) {
	var best []string
	var predicate catalog.Predicate
	found := false
	for _, p := range where {
		if p.Column != column || p.Value.Null() || !catalog.RangeOp(p.Op) {
			continue
		}
		candidates := tree.Candidates(p.Op, p.Value, at)
		if !found || len(candidates) < len(best) {
			best, predicate, found = candidates, p, true
		}
	}
	return best, predicate, found
}

// impliedByQuery proves WHERE ⇒ condition. The query is a conjunction, so the
// condition follows when each condition predicate is implied by at least one
// query predicate. Predicates are never treated as equivalent merely because
// they share a column or type.
func impliedByQuery(where, condition []catalog.Predicate) bool {
	for _, c := range condition {
		proven := false
		for _, q := range where {
			if implies(q, c) {
				proven = true
				break
			}
		}
		if !proven {
			return false
		}
	}
	return true
}

// implies reports whether q holding guarantees c holding for the same row.
// The proof is deliberately conservative: anything that cannot be shown is
// treated as unproven so the planner falls back to a scan.
func implies(q, c catalog.Predicate) bool {
	if q.Column != c.Column {
		return false
	}
	if c.Op == "not_null" {
		return q.Op != "is_null"
	}
	if c.Op == "is_null" {
		return q.Op == "is_null"
	}
	if q.Op == "is_null" || q.Op == "not_null" || q.Value.Null() || c.Value.Null() {
		return false
	}
	order, err := q.Value.Compare(c.Value)
	if err != nil {
		return false
	}
	switch c.Op {
	case "=":
		return q.Op == "=" && order == 0
	case "!=":
		switch q.Op {
		case "=":
			return order != 0
		case "!=":
			return order == 0
		case "<":
			// x < a implies x != b when a <= b.
			return order <= 0
		case "<=":
			// x <= a implies x != b when a < b.
			return order < 0
		case ">":
			// x > a implies x != b when a >= b.
			return order >= 0
		case ">=":
			// x >= a implies x != b when a > b.
			return order > 0
		}
		return false
	case ">":
		switch q.Op {
		case "=":
			return order > 0
		case ">":
			return order >= 0
		case ">=":
			return order > 0
		}
		return false
	case ">=":
		switch q.Op {
		case "=", ">", ">=":
			return order >= 0
		}
		return false
	case "<":
		switch q.Op {
		case "=":
			return order < 0
		case "<":
			return order <= 0
		case "<=":
			return order < 0
		}
		return false
	case "<=":
		switch q.Op {
		case "=", "<", "<=":
			return order <= 0
		}
		return false
	}
	return false
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }
