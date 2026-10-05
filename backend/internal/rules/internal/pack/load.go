package pack

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
)

// Files every pack has besides its rule files.
const (
	metaFile = "pack.json"
	// TextFile holds each rule's wording, copied verbatim from the library document.
	TextFile = "text.json"
)

// Load reads, decodes and validates a pack from its directory: pack.json, text.json and one or
// more rule files (every other *.json). Unknown fields are rejected everywhere.
//
// parents are the packs the pack's templates and inherited rules refer to (ADR-0007). Their
// checksums are part of the pack's checksum, so a changed parent needs a new version here too.
func Load(fsys fs.FS, parents ...*Pack) (*Pack, error) {
	names, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	sum := sha256.New()
	var p Pack
	var texts map[string]Text
	var rules []Rule
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		sum.Write([]byte(name))
		sum.Write([]byte{0})
		sum.Write(raw)
		switch name {
		case metaFile:
			err = decode(raw, &p)
		case TextFile:
			err = decode(raw, &texts)
		default:
			var f struct {
				Rules []Rule `json:"rules"`
			}
			err = decode(raw, &f)
			rules = append(rules, f.Rules...)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrInvalid, path.Base(name), err)
		}
	}
	if p.ID == "" {
		return nil, fmt.Errorf("%w: %s is missing", ErrInvalid, metaFile)
	}
	byID := map[string]*Pack{}
	for _, parent := range parents {
		byID[parent.ID] = parent
	}
	var errs []error
	seen := map[string]bool{}
	for i := range rules {
		if rules[i].Template != nil {
			if err := applyTemplate(&rules[i], byID); err != nil {
				errs = append(errs, err)
			}
		}
		t, ok := texts[rules[i].ID]
		if !ok {
			errs = append(errs, fmt.Errorf("%w: %s: no wording in %s", ErrInvalid, rules[i].ID, TextFile))
			continue
		}
		if t.Name == "" || t.Description == "" || t.EngineeringLogic == "" {
			errs = append(errs, fmt.Errorf("%w: %s: name, description and engineering logic are required", ErrInvalid, rules[i].ID))
		}
		rules[i].Text = t
		seen[rules[i].ID] = true
	}
	for id := range texts {
		if !seen[id] {
			errs = append(errs, fmt.Errorf("%w: %s has wording but no rule", ErrInvalid, id))
		}
	}
	used := map[string]bool{}
	for _, in := range p.Inherits {
		r, err := inherit(in, byID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		rules = append(rules, r)
		used[in.Pack] = true
	}
	for i := range rules {
		if rules[i].Template != nil {
			used[rules[i].Template.Pack] = true
		}
	}
	for _, id := range slices.Sorted(maps.Keys(used)) {
		if parent, ok := byID[id]; ok {
			sum.Write([]byte("parent:" + id + "@" + parent.Version + ":" + parent.Checksum))
		}
	}
	slices.SortFunc(rules, func(a, b Rule) int { return strings.Compare(a.ID, b.ID) })
	p.Rules = rules
	p.Checksum = hex.EncodeToString(sum.Sum(nil))
	if err := errors.Join(append(errs, Validate(&p))...); err != nil {
		return nil, err
	}
	return &p, nil
}

// parentRule finds a rule of a parent pack.
func parentRule(ref Ref, parents map[string]*Pack) (*Rule, error) {
	parent, ok := parents[ref.Pack]
	if !ok {
		return nil, fmt.Errorf("%w: pack %q is not loaded before this one", ErrInvalid, ref.Pack)
	}
	i := slices.IndexFunc(parent.Rules, func(r Rule) bool { return r.ID == ref.Rule })
	if i < 0 {
		return nil, fmt.Errorf("%w: pack %s has no rule %s", ErrInvalid, ref.Pack, ref.Rule)
	}
	return &parent.Rules[i], nil
}

// clone deep-copies a rule's definition (its wording is copied by the caller).
func clone(r *Rule) (Rule, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return Rule{}, err
	}
	var out Rule
	err = json.Unmarshal(raw, &out)
	return out, err
}

// applyTemplate copies the parent's logic into a "Param of X" rule and applies its overrides.
// The rule's own metadata (group, regulators, sources, defaults, tags) is kept.
func applyTemplate(r *Rule, parents map[string]*Pack) error {
	if r.Trigger != nil || r.Condition != nil || len(r.Branches) > 0 || r.Governance != nil ||
		len(r.Parameters) > 0 || len(r.Inputs) > 0 || r.Outcome.Reason != "" {
		return fmt.Errorf("%w: %s: a templated rule states only its overrides, not logic", ErrInvalid, r.ID)
	}
	parent, err := parentRule(*r.Template, parents)
	if err != nil {
		return fmt.Errorf("%s: template: %w", r.ID, err)
	}
	c, err := clone(parent)
	if err != nil {
		return err
	}
	r.Inputs, r.Trigger, r.Condition, r.Branches = c.Inputs, c.Trigger, c.Condition, c.Branches
	r.Outcome, r.Governance, r.Parameters = c.Outcome, c.Governance, c.Parameters
	return override(r, r.Overrides)
}

// inherit copies a parent rule under its own ID, with the country's licence types and settings.
func inherit(in Inherit, parents map[string]*Pack) (Rule, error) {
	parent, err := parentRule(Ref{Pack: in.Pack, Rule: in.Rule}, parents)
	if err != nil {
		return Rule{}, fmt.Errorf("inherits %s: %w", in.Rule, err)
	}
	r, err := clone(parent)
	if err != nil {
		return Rule{}, err
	}
	r.Text = parent.Text
	r.InheritedFrom = in.Pack
	r.AppliesTo = in.AppliesTo
	r.Overrides = in.Overrides
	return r, override(&r, in.Overrides)
}

// override applies setting overrides. Restating a money setting in another currency drops its
// default unless the override gives one: an amount in one currency is never reused in another.
func override(r *Rule, ov map[string]Override) error {
	for _, name := range slices.Sorted(maps.Keys(ov)) {
		o := ov[name]
		i := slices.IndexFunc(r.Parameters, func(p Parameter) bool { return p.Name == name })
		if i < 0 {
			return fmt.Errorf("%w: %s: override of unknown setting %q", ErrInvalid, r.ID, name)
		}
		p := &r.Parameters[i]
		if o.Currency != "" && o.Currency != p.Currency {
			p.Currency, p.Default = o.Currency, nil
		}
		if o.Label != "" {
			p.Label = o.Label
		}
		if o.Min != nil {
			p.Min = *o.Min
		}
		if o.Max != nil {
			p.Max = *o.Max
		}
		if o.Default != nil {
			v := *o.Default
			p.Default = &v
		}
	}
	return nil
}

func decode(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data")
	}
	return nil
}
