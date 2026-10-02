package pack

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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
func Load(fsys fs.FS) (*Pack, error) {
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
	var errs []error
	seen := map[string]bool{}
	for i := range rules {
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
	slices.SortFunc(rules, func(a, b Rule) int { return strings.Compare(a.ID, b.ID) })
	p.Rules = rules
	p.Checksum = hex.EncodeToString(sum.Sum(nil))
	if err := errors.Join(append(errs, Validate(&p))...); err != nil {
		return nil, err
	}
	return &p, nil
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
