package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/textpdf"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/matrix"
)

// Export formats.
const (
	ExportCSV = "csv"
	ExportPDF = "pdf"
)

// ClockMatrix returns the library's side-by-side comparison of the countries' legal clocks and
// thresholds. Requires compliance:read.
func (s *Service) ClockMatrix(ctx context.Context) ([]matrix.Row, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return nil, err
	}
	return matrix.Rows()
}

// exportPack is one country's library as exported: its pack and rules as the caller sees them.
type exportPack struct {
	pack  PackView
	rules []RuleView
}

// Export writes a country's rule library (or every country's, for "") in plain English, as CSV
// or PDF, with the settings in use in the caller's environment, provenance, caveats and
// sources. Audited. Requires compliance:read.
func (s *Service) Export(ctx context.Context, jurisdiction, format string, now time.Time) (data []byte, filename string, err error) {
	if format != ExportCSV && format != ExportPDF {
		return nil, "", fmt.Errorf("%w: format must be csv or pdf", ErrInvalidInput)
	}
	scope, err := readScope(ctx)
	if err != nil {
		return nil, "", err
	}
	codes := []string{jurisdiction}
	if jurisdiction == "" {
		codes = nil
		packs, err := s.Packs(ctx)
		if err != nil {
			return nil, "", err
		}
		for _, p := range packs {
			codes = append(codes, p.Jurisdiction)
		}
	}
	var packs []exportPack
	for _, code := range codes {
		pk, err := s.Pack(ctx, code)
		if err != nil {
			return nil, "", err
		}
		rules, err := s.Rules(ctx, Filter{Jurisdiction: pk.Jurisdiction})
		if err != nil {
			return nil, "", err
		}
		packs = append(packs, exportPack{pack: pk, rules: rules})
	}
	units, err := s.minorUnits(ctx)
	if err != nil {
		return nil, "", err
	}
	env := string(scope.EnvironmentKind)
	name := "all-countries"
	if jurisdiction != "" {
		name = strings.ToLower(jurisdiction)
	}
	filename = fmt.Sprintf("riskos-rules-%s-%s.%s", name, now.UTC().Format("2006-01-02"), format)
	if format == ExportCSV {
		data, err = exportCSV(packs, units, env)
	} else {
		data = exportPDF(packs, units, env, now)
	}
	if err != nil {
		return nil, "", err
	}
	total := 0
	for _, p := range packs {
		total += len(p.rules)
	}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		target := jurisdiction
		if target == "" {
			target = "all"
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "compliance_library.exported", ResourceType: "rule_pack", ResourceID: target,
			Outcome: audit.OutcomeSuccess, Metadata: map[string]any{"format": format, "rules": total},
		})
	})
	return data, filename, err
}

// minorUnits are the decimal places of each currency: the jurisdictions' and US dollars'.
func (s *Service) minorUnits(ctx context.Context) (map[string]int, error) {
	out := map[string]int{"USD": 2}
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		list, err := s.store.Jurisdictions(ctx)
		for _, j := range list {
			out[j.Currency] = int(j.MinorUnit)
		}
		return err
	})
	return out, err
}

var exportColumns = []string{ //nolint:gochecknoglobals // fixed column order
	"Country", "Rule", "Name", "Type", "What we look for", "Action or points", "Provenance", "Lineage",
	"Default", "Status", "Applies to your institution", "Settings in use", "Verification", "Caveat",
	"Regulators", "Sources", "Library basis",
}

func exportCSV(packs []exportPack, units map[string]int, env string) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	header := append([]string(nil), exportColumns...)
	header[9] = "Status in " + env
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, p := range packs {
		for i := range p.rules {
			r := &p.rules[i]
			if err := w.Write([]string{
				p.pack.Name, r.ID, r.Text.Name, kindText(r.Kind), r.Text.Description, secondText(r),
				provenanceText(r), lineageText(r), defaultText(r), modeText(r.Config.Mode), appliesText(r),
				settingsText(r, units), verificationText(r.Verification), r.Caveat, strings.Join(r.Regulators, ", "),
				sourcesText(p.pack.Pack, r.Sources), r.Text.Basis,
			}); err != nil {
				return nil, err
			}
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

func exportPDF(packs []exportPack, units map[string]int, env string, now time.Time) []byte {
	d := textpdf.New()
	for _, p := range packs {
		d.Title(p.pack.Name + " rules library")
		d.Paragraph(fmt.Sprintf("Pack %s version %s. Settings and statuses as in the %s environment on %s.",
			p.pack.ID, p.pack.Version, env, now.UTC().Format("2 January 2006")))
		d.Paragraph("This is not legal advice. Rules marked as unverified, conflicting or awaiting the regulator rest " +
			"on sources a person must confirm; the open questions at the end are not resolved by RiskOS.")
		for i := range p.rules {
			r := &p.rules[i]
			d.Heading(r.ID + "  " + r.Text.Name)
			d.Paragraph(r.Text.Description)
			d.Label(secondLabel(r), secondText(r))
			d.Label("Type", kindText(r.Kind))
			d.Label("Provenance", provenanceText(r))
			d.Label("Lineage", lineageText(r))
			d.Label("Default", defaultText(r))
			d.Label("Status in "+env, modeText(r.Config.Mode))
			d.Label("Applies to your institution", appliesText(r))
			d.Label("Settings in use", settingsText(r, units))
			d.Label("Verification", verificationText(r.Verification))
			d.Label("Caveat", r.Caveat)
			d.Label("Regulators", strings.Join(r.Regulators, ", "))
			d.Label("Sources", sourcesText(p.pack.Pack, r.Sources))
			d.Label("Library basis", r.Text.Basis)
		}
		if len(p.pack.Gaps) > 0 {
			d.Heading("Open questions")
			for _, g := range p.pack.Gaps {
				d.Label(g.ID+" "+g.Title, g.Detail)
				d.Label("Treatment until settled", g.Treatment)
			}
		}
		d.Heading("Sources")
		for _, src := range p.pack.Sources {
			var links []string
			for _, l := range src.Links {
				links = append(links, l.URL)
			}
			d.Label(src.ID+" "+src.Title, strings.Join(links, " "))
		}
	}
	return d.Bytes()
}

func kindText(k pack.Kind) string {
	switch k {
	case pack.KindControl:
		return "Required control"
	case pack.KindSignal:
		return "Risk signal"
	default:
		return "Governance"
	}
}

func secondLabel(r *RuleView) string {
	if r.Kind == pack.KindSignal {
		return "Points"
	}
	return "What the system does"
}

func secondText(r *RuleView) string {
	if r.Text.Points != "" {
		return r.Text.Points
	}
	return r.Text.Action
}

func provenanceText(r *RuleView) string {
	switch r.Provenance {
	case pack.ProvenanceRegulatory:
		return "Regulatory: follows a cited law, regulation or directive"
	case pack.ProvenanceIndustry:
		return "Industry practice: a published typology with no mandated number"
	case pack.ProvenanceDesign:
		return "The library author's design: starting values, not regulation"
	}
	if r.InheritedFrom != "" {
		return "As in the " + strings.ToUpper(r.InheritedFrom) + " library"
	}
	return ""
}

func lineageText(r *RuleView) string {
	if r.InheritedFrom != "" {
		return fmt.Sprintf("Inherited from %s rule %s", strings.ToUpper(r.InheritedFrom), r.ID)
	}
	if r.Lineage == nil {
		return ""
	}
	of := strings.Join(r.Lineage.Of, ", ")
	switch r.Lineage.Kind {
	case "param":
		if of == "" {
			return "Country version of a Nigerian practice"
		}
		return "Country version of Nigeria's " + of
	case "analogue":
		if of == "" {
			return "Analogue of a Nigerian duty"
		}
		return "Analogue of Nigeria's " + of
	case "cross_reference":
		return "Cross-reference to " + of
	case "stricter":
		return "Stricter than Nigeria's " + of
	default:
		return "New for this country"
	}
}

func defaultText(r *RuleView) string {
	switch {
	case r.Defaults.Enabled:
		return "On"
	case r.OffReason == pack.OffThresholdToConfirm:
		return "Off until a threshold is confirmed"
	case r.OffReason == pack.OffVerifyFirst:
		return "Off until the legal basis is verified"
	default:
		return "Off"
	}
}

func modeText(mode string) string {
	switch mode {
	case ModeLive:
		return "Active"
	case ModeShadow:
		return "Monitoring"
	default:
		return "Off"
	}
}

func appliesText(r *RuleView) string {
	switch {
	case r.Applicable == nil:
		return "Not activated in this country"
	case *r.Applicable:
		return "Yes"
	default:
		return "No"
	}
}

func verificationText(v pack.Verification) string {
	switch v {
	case pack.VerificationConfirmed:
		return "Confirmed against the original"
	case pack.VerificationConflicting:
		return "Sources conflict"
	case pack.VerificationAwaitingRegular:
		return "Awaiting the regulator"
	default:
		return "Secondary sources only"
	}
}

func sourcesText(p *pack.Pack, ids []string) string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		title := id
		for _, src := range p.Sources {
			if src.ID == id {
				title = id + " " + src.Title
			}
		}
		out = append(out, title)
	}
	return strings.Join(out, "; ")
}

func settingsText(r *RuleView, units map[string]int) string {
	var out []string
	for _, prm := range r.Parameters {
		v := r.Value(prm)
		text := "not set"
		if v != nil {
			text = formatSetting(prm, *v, units)
		}
		out = append(out, prm.Label+": "+text)
	}
	return strings.Join(out, "; ")
}

// formatSetting writes a setting's value in its unit: money in the currency's major units,
// durations in days, hours or minutes.
func formatSetting(p pack.Parameter, v float64, units map[string]int) string {
	switch p.Type {
	case pack.ParamMoney:
		u := units[p.Currency]
		return p.Currency + " " + groupDigits(strconv.FormatFloat(v/math.Pow10(u), 'f', u, 64))
	case pack.ParamDuration:
		switch d := time.Duration(v) * time.Second; {
		case d == 0:
			return "immediately"
		case d%(24*time.Hour) == 0:
			return plural(int(d/(24*time.Hour)), "day")
		case d%time.Hour == 0:
			return plural(int(d/time.Hour), "hour")
		default:
			return plural(int(d/time.Minute), "minute")
		}
	case pack.ParamFlag:
		if v == 1 {
			return "yes"
		}
		return "no"
	case pack.ParamPercent:
		return strconv.FormatFloat(v, 'f', -1, 64) + "%"
	case pack.ParamPoints:
		return strconv.FormatFloat(v, 'f', -1, 64) + " points"
	default:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// groupDigits adds thousands separators to a decimal number.
func groupDigits(s string) string {
	whole, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if hasFrac {
		return b.String() + "." + frac
	}
	return b.String()
}
