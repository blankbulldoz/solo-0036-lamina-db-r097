package query

import (
	"lamina/internal/catalog"
	"lamina/internal/datum"
)

// Predicate is the shared typed comparison used by requests, index
// conditions and residual checks.
type Predicate = catalog.Predicate
type Ordering struct {
	Column string `json:"column"`
	Desc   bool   `json:"desc"`
}
type Aggregate struct {
	Function string `json:"function"`
	Column   string `json:"column,omitempty"`
}
type Request struct {
	Relation    string      `json:"relation"`
	Columns     []string    `json:"columns,omitempty"`
	Where       []Predicate `json:"where,omitempty"`
	Order       []Ordering  `json:"order,omitempty"`
	Limit       int         `json:"limit,omitempty"`
	At          *uint64     `json:"at,omitempty"`
	Transaction string      `json:"transaction,omitempty"`
	Aggregate   *Aggregate  `json:"aggregate,omitempty"`
	ForceScan   bool        `json:"force_scan,omitempty"`
}
type Plan struct {
	Access     string              `json:"access"`
	Index      string              `json:"index,omitempty"`
	Candidates int                 `json:"candidates"`
	Snapshot   uint64              `json:"snapshot"`
	Residuals  int                 `json:"residuals"`
	Reason     string              `json:"reason,omitempty"`
	Condition  []catalog.Predicate `json:"condition,omitempty"`
	Considered []Consideration     `json:"considered,omitempty"`
}
type Result struct {
	Rows      []datum.Row `json:"rows"`
	Plan      Plan        `json:"plan"`
	Scanned   int         `json:"scanned"`
	Matched   int         `json:"matched"`
	Truncated bool        `json:"truncated"`
}
