package catalog

import (
	"fmt"
	"lamina/internal/datum"
	"strings"
)

// Predicate is a single typed comparison used both by query requests and by
// partial-index conditions. Conditions are deliberately small: an index may
// declare a conjunction of these predicates and nothing else.
type Predicate struct {
	Column string      `json:"column"`
	Op     string      `json:"op"`
	Value  datum.Value `json:"value"`
}

func Comparison(op string) bool {
	switch op {
	case "=", "!=", "<", ">", "<=", ">=":
		return true
	}
	return false
}

// RangeOp reports whether op can be answered directly from the ordered index.
// "!=" is a comparison but not a contiguous range, so it cannot drive an index
// range; treating it as one would return an empty candidate set and lose rows.
func RangeOp(op string) bool {
	switch op {
	case "=", "<", ">", "<=", ">=":
		return true
	}
	return false
}

func ValidOp(op string) bool {
	return Comparison(op) || op == "is_null" || op == "not_null"
}

// Holds applies the database's null semantics: ordinary comparisons never
// match a null operand, while is_null/not_null inspect presence directly.
func (p Predicate) Holds(row datum.Row) bool {
	v := row[p.Column]
	switch p.Op {
	case "is_null":
		return v.Null()
	case "not_null":
		return !v.Null()
	}
	if v.Null() || p.Value.Null() {
		return false
	}
	n, err := v.Compare(p.Value)
	if err != nil {
		return false
	}
	switch p.Op {
	case "=":
		return n == 0
	case "!=":
		return n != 0
	case "<":
		return n < 0
	case ">":
		return n > 0
	case "<=":
		return n <= 0
	case ">=":
		return n >= 0
	}
	return false
}

func HoldsAll(row datum.Row, predicates []Predicate) bool {
	for _, p := range predicates {
		if !p.Holds(row) {
			return false
		}
	}
	return true
}

func (p Predicate) String() string {
	switch p.Op {
	case "is_null":
		return p.Column + " is null"
	case "not_null":
		return p.Column + " is not null"
	}
	return p.Column + " " + p.Op + " " + literal(p.Value)
}

func literal(v datum.Value) string {
	switch v.Kind {
	case "int":
		return fmt.Sprintf("%d", v.Int)
	case "text":
		return "'" + strings.ReplaceAll(v.Text, "'", "''") + "'"
	case "bool":
		if v.Bool {
			return "true"
		}
		return "false"
	}
	return "null"
}

func Describe(predicates []Predicate) string {
	if len(predicates) == 0 {
		return "all rows"
	}
	parts := make([]string, 0, len(predicates))
	for _, p := range predicates {
		parts = append(parts, p.String())
	}
	return strings.Join(parts, " and ")
}

// ValidateCondition checks the bounded partial-index condition language.
// Values must be non-null and typed for comparisons; is_null/not_null must not
// carry a value. The bound keeps planner implication proofs decidable and
// keeps the catalog representation small.
func ValidateCondition(r Relation, conditions []Predicate) error {
	if len(conditions) > 8 {
		return fmt.Errorf("index condition supports at most 8 predicates")
	}
	for _, p := range conditions {
		c, ok := r.Column(p.Column)
		if !ok {
			return fmt.Errorf("unknown index condition column %s", p.Column)
		}
		if !ValidOp(p.Op) {
			return fmt.Errorf("unsupported index condition comparison %s", p.Op)
		}
		if p.Op == "is_null" || p.Op == "not_null" {
			if !p.Value.Null() {
				return fmt.Errorf("index condition %s takes no value", p.Op)
			}
			continue
		}
		if p.Value.Null() {
			return fmt.Errorf("index condition %s requires a non-null value", p.Column)
		}
		if p.Value.Kind != c.Type {
			return fmt.Errorf("index condition type mismatch for %s", p.Column)
		}
	}
	return nil
}
