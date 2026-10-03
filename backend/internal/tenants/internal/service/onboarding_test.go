package service

import (
	"errors"
	"testing"
)

func complete() Answers {
	yes := true
	return Answers{
		LegalName: "Acme Payments Ltd", Country: "ng", RegistrationNumber: "RC1", Website: "https://acme.example",
		Industry: "payments", Regulated: &yes, Regulator: "CBN", LicenceNumber: "PSP-1", AMLProgram: &yes,
		ComplianceOfficerName: "Grace", ComplianceOfficerEmail: "Grace@Acme.Example", MonitoredEvents: []string{"payments"},
		MonthlyVolume: "100k_1m", RegionsServed: []string{"ng", "GH"}, GoLive: "now", DataProtection: []string{"ndpr"},
		DataResidency: "africa", AcceptTerms: true, AcceptDPA: true,
	}
}

func TestAnswersNormalizeAndMissing(t *testing.T) {
	a := complete()
	if err := a.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if a.Country != "NG" || a.RegionsServed[0] != "NG" || a.ComplianceOfficerEmail != "grace@acme.example" {
		t.Fatalf("not normalized: %+v", a)
	}
	if m := a.Missing(); len(m) != 0 {
		t.Fatalf("complete answers miss %v", m)
	}

	// A regulator and licence are needed only when regulated.
	no := false
	a.Regulated, a.Regulator, a.LicenceNumber = &no, "", ""
	if m := a.Missing(); len(m) != 0 {
		t.Fatalf("unregulated business still misses %v", m)
	}
	a.Regulated = nil
	if m := a.Missing(); len(m) != 1 || m[0] != "regulated" {
		t.Fatalf("missing = %v, want [regulated]", m)
	}
	// Terms must be accepted.
	a = complete()
	a.AcceptDPA = false
	if m := a.Missing(); len(m) != 1 || m[0] != "accept_dpa" {
		t.Fatalf("missing = %v, want [accept_dpa]", m)
	}
	// Everything is missing from an empty draft, which is still valid to save.
	var empty Answers
	if err := empty.normalize(); err != nil {
		t.Fatalf("an empty draft must save: %v", err)
	}
	if len(empty.Missing()) < 15 {
		t.Fatalf("empty draft misses only %v", empty.Missing())
	}
}

func TestAnswersRejectBadValues(t *testing.T) {
	for name, mutate := range map[string]func(*Answers){
		"country":       func(a *Answers) { a.Country = "Nigeria" },
		"website":       func(a *Answers) { a.Website = "javascript:alert(1)" },
		"industry":      func(a *Answers) { a.Industry = "weapons" },
		"officer email": func(a *Answers) { a.ComplianceOfficerEmail = "not an email" },
		"volume":        func(a *Answers) { a.MonthlyVolume = "lots" },
		"events dup":    func(a *Answers) { a.MonitoredEvents = []string{"payments", "payments"} },
		"events value":  func(a *Answers) { a.MonitoredEvents = []string{"everything"} },
		"regions":       func(a *Answers) { a.RegionsServed = []string{"Nigeria"} },
		"privacy":       func(a *Answers) { a.DataProtection = []string{"hipaa"} },
		"residency":     func(a *Answers) { a.DataResidency = "mars" },
		"long name":     func(a *Answers) { a.LegalName = string(make([]rune, 201)) },
	} {
		a := complete()
		mutate(&a)
		if err := a.normalize(); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}
