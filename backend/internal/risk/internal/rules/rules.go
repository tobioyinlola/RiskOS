// Package rules implements the customer rules language (ARCHITECTURE.md §10, spec §9):
// structured JSON conditions over a fixed catalog of typed facts, combined with all/any/not.
//
// There is no eval, no scripting, no regular expressions and no customer code. Rules are parsed
// and compiled once per version; size and depth limits bound evaluation cost linearly, so a
// pathological rule cannot slow the synchronous decision path.
package rules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Limits on a rule's condition tree.
const (
	MaxDepth     = 6
	MaxNodes     = 50
	MaxListItems = 100
)

// ErrInvalid wraps every validation failure; messages are safe to show to the rule author.
var ErrInvalid = errors.New("invalid rule condition")

// Type of a fact.
type Type int

// Fact types.
const (
	Number Type = iota + 1
	String
	Bool
)

func (t Type) String() string {
	switch t {
	case Number:
		return "number"
	case String:
		return "string"
	default:
		return "boolean"
	}
}

// Fields is the catalog of facts a rule can reference, with their types. Event facts come from
// the normalized event; feature.* facts come from the feature store at evaluation time.
func Fields() map[string]Type {
	return map[string]Type{
		"event.type":                       String,
		"context.country":                  String,
		"transaction.amount":               Number,
		"transaction.currency":             String,
		"transaction.direction":            String,
		"transaction.channel":              String,
		"login.outcome":                    String,
		"login.method":                     String,
		"beneficiary.country":              String,
		"profile.credential_change":        Bool,
		"feature.new_user":                 Bool,
		"feature.new_device":               Bool,
		"feature.new_beneficiary":          Bool,
		"feature.new_country":              Bool,
		"feature.recent_credential_change": Bool,
		"feature.account_txn_1m":           Number,
		"feature.account_txn_1h":           Number,
		"feature.account_txn_24h":          Number,
		"feature.failed_logins_15m":        Number,
		"feature.device_users":             Number,
		"feature.beneficiary_senders_24h":  Number,
		"feature.amount_to_mean_ratio":     Number,
	}
}

// Operators returns the operators valid for a fact type.
func Operators(t Type) []string {
	switch t {
	case Number:
		return []string{"eq", "ne", "gt", "gte", "lt", "lte", "in", "not_in"}
	case String:
		return []string{"eq", "ne", "in", "not_in"}
	default:
		return []string{"eq", "ne"}
	}
}

// Condition is the JSON form of a condition tree node. Exactly one of All, Any, Not or Field is set.
type Condition struct {
	All   []Condition     `json:"all,omitempty"`
	Any   []Condition     `json:"any,omitempty"`
	Not   *Condition      `json:"not,omitempty"`
	Field string          `json:"field,omitempty"`
	Op    string          `json:"op,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// Facts are the values a compiled condition is evaluated against: float64, string or bool per
// the catalog. A missing fact (e.g. transaction.amount on a login) makes any comparison on it false.
type Facts map[string]any

// Compiled is a validated condition ready to evaluate.
type Compiled struct {
	root   node
	fields []string
}

// Fields returns the facts the condition references, sorted.
func (c Compiled) Fields() []string { return slices.Clone(c.fields) }

// Match evaluates the condition.
func (c Compiled) Match(f Facts) bool { return c.root.eval(f) }

// Compile parses and validates a JSON condition.
func Compile(raw []byte) (Compiled, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c Condition
	if err := dec.Decode(&c); err != nil {
		return Compiled{}, fmt.Errorf("%w: %s", ErrInvalid, jsonReason(err))
	}
	if dec.More() {
		return Compiled{}, fmt.Errorf("%w: trailing data after the condition", ErrInvalid)
	}
	b := &builder{catalog: Fields(), used: map[string]bool{}}
	root, err := b.build(c, "condition", 1)
	if err != nil {
		return Compiled{}, err
	}
	fields := make([]string, 0, len(b.used))
	for f := range b.used {
		fields = append(fields, f)
	}
	slices.Sort(fields)
	return Compiled{root: root, fields: fields}, nil
}

func jsonReason(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, "json: "); i >= 0 {
		msg = msg[i+len("json: "):]
	}
	return msg
}

type builder struct {
	catalog map[string]Type
	nodes   int
	used    map[string]bool
}

func (b *builder) build(c Condition, path string, depth int) (node, error) {
	b.nodes++
	if b.nodes > MaxNodes {
		return nil, fmt.Errorf("%w: more than %d nodes", ErrInvalid, MaxNodes)
	}
	if depth > MaxDepth {
		return nil, fmt.Errorf("%w: %s: nested deeper than %d levels", ErrInvalid, path, MaxDepth)
	}
	set := 0
	for _, isSet := range []bool{c.All != nil, c.Any != nil, c.Not != nil, c.Field != ""} {
		if isSet {
			set++
		}
	}
	if set != 1 {
		return nil, fmt.Errorf("%w: %s: must have exactly one of all, any, not or field", ErrInvalid, path)
	}
	switch {
	case c.All != nil || c.Any != nil:
		list, kind := c.All, "all"
		if c.Any != nil {
			list, kind = c.Any, "any"
		}
		if c.Op != "" || c.Value != nil {
			return nil, fmt.Errorf("%w: %s: a group cannot have op or value", ErrInvalid, path)
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("%w: %s.%s: must not be empty", ErrInvalid, path, kind)
		}
		children := make([]node, len(list))
		for i, child := range list {
			n, err := b.build(child, fmt.Sprintf("%s.%s[%d]", path, kind, i), depth+1)
			if err != nil {
				return nil, err
			}
			children[i] = n
		}
		if kind == "all" {
			return allNode(children), nil
		}
		return anyNode(children), nil
	case c.Not != nil:
		if c.Op != "" || c.Value != nil {
			return nil, fmt.Errorf("%w: %s: not cannot have op or value", ErrInvalid, path)
		}
		n, err := b.build(*c.Not, path+".not", depth+1)
		if err != nil {
			return nil, err
		}
		return notNode{n}, nil
	default:
		return b.comparison(c, path)
	}
}

func (b *builder) comparison(c Condition, path string) (node, error) {
	typ, ok := b.catalog[c.Field]
	if !ok {
		return nil, fmt.Errorf("%w: %s: unknown field %q", ErrInvalid, path, c.Field)
	}
	if !slices.Contains(Operators(typ), c.Op) {
		return nil, fmt.Errorf("%w: %s: operator %q is not valid for %s field %q (use one of %s)",
			ErrInvalid, path, c.Op, typ, c.Field, strings.Join(Operators(typ), ", "))
	}
	if c.Value == nil {
		return nil, fmt.Errorf("%w: %s: value is required", ErrInvalid, path)
	}
	b.used[c.Field] = true
	if c.Op == "in" || c.Op == "not_in" {
		var items []json.RawMessage
		if err := json.Unmarshal(c.Value, &items); err != nil || len(items) == 0 || len(items) > MaxListItems {
			return nil, fmt.Errorf("%w: %s: value must be a list of 1-%d %s values", ErrInvalid, path, MaxListItems, typ)
		}
		values := make([]any, len(items))
		for i, item := range items {
			v, err := decodeValue(item, typ)
			if err != nil {
				return nil, fmt.Errorf("%w: %s.value[%d]: %w", ErrInvalid, path, i, err)
			}
			values[i] = v
		}
		return inNode{field: c.Field, values: values, negate: c.Op == "not_in"}, nil
	}
	v, err := decodeValue(c.Value, typ)
	if err != nil {
		return nil, fmt.Errorf("%w: %s.value: %w", ErrInvalid, path, err)
	}
	return cmpNode{field: c.Field, op: c.Op, value: v}, nil
}

func decodeValue(raw json.RawMessage, typ Type) (any, error) {
	switch typ {
	case Number:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, errors.New("must be a number")
		}
		return f, nil
	case String:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || len(s) > 256 {
			return nil, errors.New("must be a string of at most 256 characters")
		}
		return s, nil
	default:
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, errors.New("must be true or false")
		}
		return v, nil
	}
}

type node interface{ eval(Facts) bool }

type allNode []node

func (n allNode) eval(f Facts) bool {
	for _, c := range n {
		if !c.eval(f) {
			return false
		}
	}
	return true
}

type anyNode []node

func (n anyNode) eval(f Facts) bool {
	for _, c := range n {
		if c.eval(f) {
			return true
		}
	}
	return false
}

type notNode struct{ n node }

func (n notNode) eval(f Facts) bool { return !n.n.eval(f) }

type cmpNode struct {
	field string
	op    string
	value any
}

func (n cmpNode) eval(f Facts) bool {
	got, ok := f[n.field]
	if !ok {
		return false
	}
	switch want := n.value.(type) {
	case float64:
		g, ok := got.(float64)
		if !ok {
			return false
		}
		switch n.op {
		case "eq":
			return g == want
		case "ne":
			return g != want
		case "gt":
			return g > want
		case "gte":
			return g >= want
		case "lt":
			return g < want
		case "lte":
			return g <= want
		}
	default:
		switch n.op {
		case "eq":
			return got == want
		case "ne":
			return got != want
		}
	}
	return false
}

type inNode struct {
	field  string
	values []any
	negate bool
}

func (n inNode) eval(f Facts) bool {
	got, ok := f[n.field]
	if !ok {
		return false
	}
	return slices.Contains(n.values, got) != n.negate
}
