package interp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
)

// ghs, rwf and usd are amounts in cedis (minor units: pesewas), Rwandan francs (no minor unit)
// and US dollars (cents).
func ghs(cedis float64) interp.Money  { return interp.Money{Minor: cedis * 100, Currency: "GHS"} }
func rwf(francs float64) interp.Money { return interp.Money{Minor: francs, Currency: "RWF"} }
func usd(dollars float64) interp.Money {
	return interp.Money{Minor: dollars * 100, Currency: "USD"}
}

// countryCases are written from the library's WHEN/IF/THEN logic
// (docs/compliance/GHANA_RWANDA_RULES_LIBRARY.md sections 4.2 and 5.2), with its amounts, at
// each threshold: the generated tests in countries_test.go take the comparison from the pack,
// these take it from the library, so a pack that says "above" where the library says "at or
// above" fails here.
var countryCases = map[string][]tc{ //nolint:gochecknoglobals // test table
	// SUM(txn.amount) + txn.amount > daily limit: Minimum GHS 3,000, Medium GHS 15,000,
	// Enhanced GHS 25,000. The window's sum includes the event being decided.
	"GH-REG-01": {
		{name: "Minimum tier one pesewa over GHS 3,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(3000.01)}, want: fired},
		{name: "Minimum tier at GHS 3,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(3000)}, want: clr},
		{name: "Medium tier one pesewa over GHS 15,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "medium"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(15000.01)}, want: fired},
		{name: "Medium tier at GHS 15,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "medium"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(15000)}, want: clr},
		{name: "Enhanced tier one pesewa over GHS 25,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "enhanced"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(25000.01)}, want: fired},
		{name: "Enhanced tier at GHS 25,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "enhanced"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(25000)}, want: clr},
		{name: "tier unknown", facts: F{"account.type": "mobile_money_wallet"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(30000)}, want: notEv, missing: "fact:wallet.kyc_tier"},
		{name: "not a mobile money wallet", facts: F{"account.type": "current"}, want: notApp},
	},
	// wallet.balance + txn.amount > balance cap: Minimum GHS 5,000, Medium GHS 40,000, Enhanced
	// GHS 75,000.
	"GH-REG-02": {
		{name: "Minimum tier over GHS 5,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum", "wallet.balance_after": ghs(5000.01)}, want: fired},
		{name: "Minimum tier at GHS 5,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum", "wallet.balance_after": ghs(5000)}, want: clr},
		{name: "Medium tier over GHS 40,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "medium", "wallet.balance_after": ghs(40000.01)}, want: fired},
		{name: "Enhanced tier at GHS 75,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "enhanced", "wallet.balance_after": ghs(75000)}, want: clr},
		{name: "Enhanced tier over GHS 75,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "enhanced", "wallet.balance_after": ghs(75000.01)}, want: fired},
		{name: "balance unknown", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum"}, want: notEv, missing: "fact:wallet.balance_after"},
	},
	// Monthly: Minimum GHS 10,000; Medium and Enhanced NULL (to be confirmed) until set.
	"GH-REG-03": {
		{name: "Minimum tier over GHS 10,000 in the month", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(10000.01)}, want: fired},
		{name: "Minimum tier at GHS 10,000", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(10000)}, want: clr},
		{name: "Medium tier: the limit is the institution's to enter", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "medium"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(1_000_000)}, want: notEv, missing: "setting:monthly_medium"},
		{name: "Medium tier once entered", params: map[string]float64{"monthly_medium": 5_000_000}, facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "medium"},
			aggs: A{"sum(transaction_any.txn.amount) by account where": ghs(50000.01)}, want: fired},
	},
	// txn.amount >= GHS 50,000 (bank, securities) or GHS 20,000 (SDI, forex bureau), domestic cash.
	"GH-REG-04": {
		{name: "bank at GHS 50,000", facts: F{"txn.domestic": true, "institution.licence_type": "bank", "txn.amount": ghs(50000)}, want: fired},
		{name: "bank one pesewa under", facts: F{"txn.domestic": true, "institution.licence_type": "bank", "txn.amount": ghs(49999.99)}, want: clr},
		{name: "SDI at GHS 20,000", facts: F{"txn.domestic": true, "institution.licence_type": "sdi", "txn.amount": ghs(20000)}, want: fired},
		{name: "forex bureau one pesewa under", facts: F{"txn.domestic": true, "institution.licence_type": "forex_bureau", "txn.amount": ghs(19999.99)}, want: clr},
		{name: "bank at the SDI threshold", facts: F{"txn.domestic": true, "institution.licence_type": "bank", "txn.amount": ghs(20000)}, want: clr},
		{name: "amount unknown", facts: F{"txn.domestic": true, "institution.licence_type": "bank"}, want: notEv},
		{name: "cross-border cash", facts: F{"txn.domestic": false}, want: notApp},
	},
	// txn.amount_usd_equiv >= USD 1,000. No recorded rate, no US dollar amount: not evaluated.
	"GH-REG-05": {
		{name: "at USD 1,000", facts: F{"txn.amount_usd": usd(1000)}, want: fired},
		{name: "one cent under", facts: F{"txn.amount_usd": usd(999.99)}, want: clr},
		{name: "no exchange rate recorded", facts: F{"txn.amount": ghs(1_000_000)}, want: notEv, missing: "fact:txn.amount_usd"},
	},
	// Every recorded suspicion starts the STR clock; no minimum amount.
	"GH-REG-06": {
		{name: "suspicion recorded", want: fired},
		{name: "not a suspicion", kinds: []string{"login"}, want: notApp},
	},
	"GH-REG-07": {
		{name: "customer is a PEP", facts: F{"customer.is_pep": true}, want: fired},
		{name: "a beneficial owner is a PEP", facts: F{"customer.is_pep": false, "customer.has_pep_owner": true}, want: fired},
		{name: "no PEP", facts: F{"customer.is_pep": false, "customer.has_pep_owner": false}, want: clr},
		{name: "PEP screening missing", want: notEv},
	},
	// score >= match threshold AND (match confirmed OR auto-block on strong match).
	"GH-REG-08": {
		{name: "confirmed match at the threshold", facts: F{"screening.sanctions_score": 90.0, "screening.sanctions_confirmed": true}, want: fired},
		{name: "confirmed but below the threshold", facts: F{"screening.sanctions_score": 89.999, "screening.sanctions_confirmed": true}, want: clr},
		{name: "strong match not yet reviewed", facts: F{"screening.sanctions_score": 95.0, "screening.sanctions_confirmed": false}, want: clr},
		{name: "strong match with auto-block on", params: map[string]float64{"auto_block_strong_match": 1},
			facts: F{"screening.sanctions_score": 95.0, "screening.sanctions_confirmed": false}, want: fired},
		{name: "screening missing", want: notEv},
	},
	// Forex bureaus, or institutions that choose to require the Ghana Card.
	"GH-REG-09": {
		{name: "forex bureau, card not verified", facts: F{"institution.licence_type": "forex_bureau", "identity.ghana_card_verified": false}, want: fired},
		{name: "forex bureau, card verified", facts: F{"institution.licence_type": "forex_bureau", "identity.ghana_card_verified": true}, want: clr},
		{name: "a bank that has not chosen to require it", facts: F{"institution.licence_type": "bank"}, want: notApp},
		{name: "a bank that requires it", params: map[string]float64{"require_ghana_card": 1},
			facts: F{"institution.licence_type": "bank", "identity.ghana_card_verified": false}, want: fired},
		{name: "verification result missing", facts: F{"institution.licence_type": "forex_bureau"}, want: notEv},
	},
	"GH-REG-10": {
		{name: "opening with identity unverified", kinds: []string{"account_opened"},
			facts: F{"event.kind": "account_opened", "kyc.identity_verified": false}, want: fired, branch: "Opening an account"},
		{name: "existing nominee account with no owner", kinds: []string{"daily_job"},
			facts: F{"event.kind": "daily_job", "kyc.identity_verified": true, "customer.name_missing": false, "account.nominal_holder_without_owner": true}, want: fired, branch: "Existing account"},
		{name: "identified, named, owned", kinds: []string{"account_opened"},
			facts: F{"event.kind": "account_opened", "kyc.identity_verified": true, "customer.name_missing": false, "account.nominal_holder_without_owner": false}, want: clr},
		{name: "identity check missing", kinds: []string{"account_opened"},
			facts: F{"event.kind": "account_opened", "customer.name_missing": false, "account.nominal_holder_without_owner": false}, want: notEv},
	},
	"GH-REG-11": {
		{name: "FIC freeze order", facts: F{"order.source": "FIC", "order.type": "freeze"}, want: fired},
		{name: "FIC suspension order", facts: F{"order.source": "FIC", "order.type": "suspension"}, want: fired},
		{name: "FIC information request", facts: F{"order.source": "FIC", "order.type": "information_request"}, want: clr},
		{name: "order from elsewhere", facts: F{"order.source": "court"}, want: notApp},
	},
	// Forex bureaus: txn.amount_usd_equiv >= USD 10,000.
	"GH-REG-12": {
		{name: "at USD 10,000", kinds: []string{"forex_buy"}, facts: F{"txn.amount_usd": usd(10000)}, want: fired},
		{name: "one cent under", kinds: []string{"forex_sell"}, facts: F{"txn.amount_usd": usd(9999.99)}, want: clr},
		{name: "no exchange rate", kinds: []string{"forex_buy"}, want: notEv, missing: "fact:txn.amount_usd"},
	},
	// The travel-rule minimum is NULL (to be confirmed): nothing runs until it is entered.
	"GH-REG-13": {
		{name: "minimum not entered", facts: F{"txn.amount_usd": usd(5000), "crypto.travel_rule_complete": false}, want: notEv, missing: "setting:travel_rule_min"},
		{name: "incomplete data at the minimum", params: map[string]float64{"travel_rule_min": 100_000, "hold_max": 86400},
			facts: F{"txn.amount_usd": usd(1000), "crypto.travel_rule_complete": false}, want: fired},
		{name: "complete data", params: map[string]float64{"travel_rule_min": 100_000, "hold_max": 86400},
			facts: F{"txn.amount_usd": usd(1000), "crypto.travel_rule_complete": true}, want: clr},
		{name: "under the minimum", params: map[string]float64{"travel_rule_min": 100_000, "hold_max": 86400},
			facts: F{"txn.amount_usd": usd(999.99), "crypto.travel_rule_complete": false}, want: clr},
	},
	// Daily cash 80% of the limit on three days in a week, the library's second test.
	"GH-TM-07": {
		{name: "three heavy days in the week", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum"},
			aggs: A{"count(transaction_any) by account where": 0.0, "days_over(transaction_any.txn.amount) by account where": 3.0}, want: fired, weight: 25},
		{name: "two heavy days", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum"},
			aggs: A{"count(transaction_any) by account where": 0.0, "days_over(transaction_any.txn.amount) by account where": 2.0}, want: clr},
		{name: "three payments just under the limit", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "medium"},
			aggs: A{"count(transaction_any) by account where": 3.0, "days_over(transaction_any.txn.amount) by account where": 0.0}, want: fired},
		{name: "daily totals unavailable", facts: F{"account.type": "mobile_money_wallet", "wallet.kyc_tier": "minimum"},
			aggs: A{"count(transaction_any) by account where": 0.0}, want: notEv},
	},
	// txn.amount_usd_equiv > USD 10,000 (not exceeding USD 10,000 needs no declaration).
	"RW-REG-01": {
		{name: "over USD 10,000, undeclared", facts: F{"txn.cash_source": "cross_border", "txn.amount_usd": usd(10000.01), "declaration.reference_present": false}, want: fired},
		{name: "at USD 10,000", facts: F{"txn.cash_source": "cross_border", "txn.amount_usd": usd(10000), "declaration.reference_present": false}, want: clr},
		{name: "declared", facts: F{"txn.cash_source": "cross_border", "txn.amount_usd": usd(50000), "declaration.reference_present": true}, want: clr},
		{name: "no exchange rate", facts: F{"txn.cash_source": "cross_border", "declaration.reference_present": false}, want: notEv, missing: "fact:txn.amount_usd"},
		{name: "domestic cash", facts: F{"txn.cash_source": "domestic"}, want: notApp},
	},
	"RW-REG-02": {
		{name: "designated party", facts: F{"screening.designation_match": true}, want: fired},
		{name: "no match", facts: F{"screening.designation_match": false}, want: clr},
		{name: "screening missing", want: notEv},
	},
	"RW-REG-03": {
		{name: "confirmed match at the threshold", facts: F{"screening.sanctions_score": 90.0, "screening.sanctions_confirmed": true}, want: fired},
		{name: "below the threshold", facts: F{"screening.sanctions_score": 89.999, "screening.sanctions_confirmed": true}, want: clr},
		{name: "strong match with auto-block on", params: map[string]float64{"auto_block_strong_match": 1},
			facts: F{"screening.sanctions_score": 90.0, "screening.sanctions_confirmed": false}, want: fired},
		{name: "screening missing", want: notEv},
	},
	"RW-REG-04": {
		{name: "suspicion recorded", want: fired},
		{name: "not a suspicion", kinds: []string{"login"}, want: notApp},
	},
	// txn.amount >= RWF 10,000,000 (placeholder from the brief).
	"RW-REG-05": {
		{name: "at RWF 10,000,000", facts: F{"txn.amount": rwf(10_000_000)}, want: fired},
		{name: "one franc under", facts: F{"txn.amount": rwf(9_999_999)}, want: clr},
		{name: "amount unknown", want: notEv},
	},
	"RW-REG-06": {
		{name: "customer is a PEP", facts: F{"customer.is_pep": true}, want: fired},
		{name: "no PEP", facts: F{"customer.is_pep": false, "customer.has_pep_owner": false}, want: clr},
		{name: "PEP screening missing", want: notEv},
	},
	// txn.amount > RWF 10,000,000; fee > RWF 20.
	"RW-REG-07": {
		{name: "one franc over the eKash cap", facts: F{"txn.rail": "ekash", "txn.amount": rwf(10_000_001), "txn.fee": rwf(0)}, want: fired, branch: "Above the eKash cap"},
		{name: "at the cap, fee at RWF 20", facts: F{"txn.rail": "ekash", "txn.amount": rwf(10_000_000), "txn.fee": rwf(20)}, want: clr},
		{name: "fee of RWF 21", facts: F{"txn.rail": "ekash", "txn.amount": rwf(1000), "txn.fee": rwf(21)}, want: fired, branch: "Fee above the ceiling"},
		{name: "fee unknown", facts: F{"txn.rail": "ekash", "txn.amount": rwf(1000)}, want: notEv},
		{name: "not an eKash payment", facts: F{"txn.rail": "swift"}, want: notApp},
	},
	// SUM(txn.amount) + txn.amount > RWF 5,000,000 a day, unless the bank authorised it.
	"RW-REG-08": {
		{name: "one franc over RWF 5,000,000", facts: F{"exception.authorised_by_bank": false},
			aggs: A{"sum(cash_withdrawal.txn.amount) by customer": rwf(5_000_001)}, want: fired},
		{name: "at RWF 5,000,000", aggs: A{"sum(cash_withdrawal.txn.amount) by customer": rwf(5_000_000)}, want: clr},
		{name: "authorised by the bank", facts: F{"exception.authorised_by_bank": true},
			aggs: A{"sum(cash_withdrawal.txn.amount) by customer": rwf(9_000_000)}, want: clr},
		{name: "authorisation not stated: the ceiling applies", aggs: A{"sum(cash_withdrawal.txn.amount) by customer": rwf(9_000_000)},
			want: fired, exemptionUnknown: true},
		{name: "daily total unavailable", want: notEv},
	},
	// COUNT(e-money accounts) >= 3 without BNR authorisation.
	"RW-REG-09": {
		{name: "a fourth account (three held)", facts: F{"account.type": "e_money_account", "customer.emoney_accounts": 3.0, "customer.bnr_authorisation": false}, want: fired},
		{name: "two held", facts: F{"account.type": "e_money_account", "customer.emoney_accounts": 2.0, "customer.bnr_authorisation": false}, want: clr},
		{name: "authorised by BNR", facts: F{"account.type": "e_money_account", "customer.emoney_accounts": 5.0, "customer.bnr_authorisation": true}, want: clr},
		{name: "count unknown", facts: F{"account.type": "e_money_account", "customer.bnr_authorisation": false}, want: notEv},
	},
	"RW-REG-10": {
		{name: "NIDA not verified", facts: F{"identity.nida_verified": false}, want: fired},
		{name: "verified and named", facts: F{"identity.nida_verified": true, "customer.name_missing": false}, want: clr},
		{name: "verification missing", facts: F{"customer.name_missing": false}, want: notEv},
	},
	"RW-REG-11": {
		{name: "no customer authentication", facts: F{"event.channel": "mobile_app", "txn.auth_method": "none"}, want: fired},
		{name: "PIN used", facts: F{"event.channel": "ussd", "txn.auth_method": "pin"}, want: clr},
		{name: "method not reported", facts: F{"event.channel": "web"}, want: notEv},
		{name: "branch counter", facts: F{"event.channel": "branch"}, want: notApp},
	},
	"RW-REG-12": {
		{name: "owner not identified", facts: F{"customer.type": "corporate", "beneficial_owner.identified": false}, want: fired},
		{name: "owner identified, documents complete", facts: F{"customer.type": "corporate", "beneficial_owner.identified": true, "beneficial_owner.documents_complete": true}, want: clr},
		{name: "ownership unknown", facts: F{"customer.type": "corporate", "beneficial_owner.documents_complete": true}, want: notEv},
		{name: "a natural person", facts: F{"customer.type": "individual"}, want: notApp},
	},
	"RW-REG-13": {
		{name: "unlicensed virtual asset provider", facts: F{"counterparty.is_vasp": true, "counterparty.vasp_licensed": false}, want: fired},
		{name: "merchant accepting virtual assets without authorisation", facts: F{"counterparty.is_vasp": false, "merchant.accepts_virtual_assets": true, "merchant.va_authorised": false}, want: fired},
		{name: "licensed provider, ordinary merchant", facts: F{"counterparty.is_vasp": true, "counterparty.vasp_licensed": true, "merchant.accepts_virtual_assets": false}, want: clr},
		{name: "register not loaded", facts: F{"counterparty.is_vasp": true, "merchant.accepts_virtual_assets": false}, want: notEv},
	},
	"RW-REG-14": {
		{name: "minimum not entered", facts: F{"txn.amount_usd": usd(5000), "crypto.travel_rule_complete": false}, want: notEv, missing: "setting:travel_rule_min"},
		{name: "incomplete data at the minimum", params: map[string]float64{"travel_rule_min": 100_000, "hold_max": 86400},
			facts: F{"txn.amount_usd": usd(1000), "crypto.travel_rule_complete": false}, want: fired},
	},
	// Significant incidents to the CSA within 24 hours; confirmed vulnerabilities within 72.
	"GH-SEC-08": {
		{name: "critical incident", facts: F{"event.kind": "incident_detected", "incident.severity": "critical"}, want: fired, branch: "Significant incident"},
		{name: "minor incident", facts: F{"event.kind": "incident_detected", "incident.severity": "minor"}, want: clr},
		{name: "confirmed vulnerability", kinds: []string{"vulnerability_confirmed"}, facts: F{"event.kind": "vulnerability_confirmed"}, want: fired, branch: "Vulnerability"},
		{name: "severity not assessed", facts: F{"event.kind": "incident_detected"}, want: notEv},
	},
	"GH-SEC-09": {
		{name: "fraud", facts: F{"incident.kind": "fraud"}, want: fired},
		{name: "material service interruption", facts: F{"incident.kind": "service_interruption"}, want: fired},
		{name: "another kind", facts: F{"incident.kind": "phishing_attempt"}, want: clr},
		{name: "kind not recorded", want: notEv},
	},
	"GH-SEC-10": {
		{name: "outsourcing planned", want: fired},
		{name: "outsourcing signed", kinds: []string{"outsourcing_signed"}, want: fired},
		{name: "not outsourcing", kinds: []string{"login"}, want: notApp},
	},
	// Audit logs are kept six years (Act 987); coverage below 100% is a gap.
	"GH-SEC-11": {
		{name: "deleting a log a second before six years", kinds: []string{"record_delete_requested"},
			facts: F{"record.type": "audit_log", "record.age": 6*365*day - 1, "audit_log.coverage": 1.0}, want: fired, branch: "Audit log deleted early"},
		{name: "deleting a log at six years", kinds: []string{"record_delete_requested"},
			facts: F{"record.type": "audit_log", "record.age": 6 * 365 * day, "audit_log.coverage": 1.0}, want: clr},
		{name: "coverage 99.9%", kinds: []string{"audit_log_check"}, facts: F{"audit_log.coverage": 0.999, "record.type": "none"}, want: fired, branch: "Gap in the audit log"},
		{name: "coverage unknown", kinds: []string{"audit_log_check"}, facts: F{"record.type": "none"}, want: notEv},
	},
	"GH-SEC-12": {
		{name: "personal data reached without authority", facts: F{"breach.personal_data": true, "breach.unauthorised_access": true}, want: fired},
		{name: "no unauthorised access", facts: F{"breach.personal_data": true, "breach.unauthorised_access": false}, want: clr},
		{name: "access not established", facts: F{"breach.personal_data": true}, want: notEv},
	},
	// NOT (consent AND safeguards); Bank of Ghana approval only when the institution turns it on.
	"GH-SEC-13": {
		{name: "no consent recorded", facts: F{"data.personal": true, "data.destination_foreign": true, "data.consent_recorded": false}, want: fired},
		{name: "consent and safeguards recorded", facts: F{"data.personal": true, "data.destination_foreign": true, "data.consent_recorded": true, "data.safeguards_recorded": true}, want: clr},
		{name: "no approval, switch on", params: map[string]float64{"require_bog_approval": 1},
			facts: F{"data.personal": true, "data.destination_foreign": true, "data.consent_recorded": true, "data.safeguards_recorded": true, "data.bog_approval": false}, want: fired},
		{name: "safeguards unknown", facts: F{"data.personal": true, "data.destination_foreign": true, "data.consent_recorded": true}, want: notEv},
		{name: "stays in Ghana", facts: F{"data.personal": true, "data.destination_foreign": false}, want: notApp},
	},
	"GH-SEC-14": {
		{name: "unlicensed provider", facts: F{"vendor.service": "cybersecurity", "vendor.csa_licence_valid": false}, want: fired},
		{name: "licensed", facts: F{"vendor.service": "cybersecurity", "vendor.csa_licence_valid": true}, want: clr},
		{name: "licence not checked", facts: F{"vendor.service": "cybersecurity"}, want: notEv},
		{name: "not a cybersecurity vendor", facts: F{"vendor.service": "catering"}, want: notApp},
	},
	"RW-SEC-02": {
		{name: "remote login without MFA", facts: F{"session.mfa_passed": false, "session.compensating_control_approved": false}, want: fired},
		{name: "written approval of a compensating control", facts: F{"session.mfa_passed": false, "session.compensating_control_approved": true}, want: clr},
		{name: "MFA passed", facts: F{"session.mfa_passed": true}, want: clr},
		{name: "MFA result missing", want: notEv},
	},
	// The BNR clock is not found: the rule flags the incident, the deadline stays to be entered.
	"RW-SEC-03": {
		{name: "major incident", facts: F{"incident.severity": "major"}, want: fired},
		{name: "moderate incident", facts: F{"incident.severity": "moderate"}, want: clr},
		{name: "severity not assessed", want: notEv},
	},
	"RW-SEC-05": {
		{name: "critical incident: immediately", facts: F{"incident.severity": "critical"}, want: fired, branch: "Critical or major incident"},
		{name: "moderate incident: monthly", facts: F{"incident.severity": "moderate"}, want: fired, branch: "Moderate incident"},
		{name: "minor incident", facts: F{"incident.severity": "minor"}, want: clr},
		{name: "severity not assessed", want: notEv},
	},
	// Penetration test every 12 months; vulnerability assessment and internal audit every 6.
	"RW-SEC-06": {
		{name: "penetration test a second over a year old", kinds: []string{"security_review_due"},
			facts: F{"event.kind": "security_review_due", "security.pentest_age": 365*day + 1, "security.vulnerability_assessment_age": 0.0, "security.internal_audit_age": 0.0}, want: fired, branch: "Penetration test due"},
		{name: "everything within its cycle", kinds: []string{"security_review_due"},
			facts: F{"event.kind": "security_review_due", "security.pentest_age": 365 * day, "security.vulnerability_assessment_age": 182 * day, "security.internal_audit_age": 182 * day}, want: clr},
		{name: "internal audit overdue", kinds: []string{"security_review_due"},
			facts: F{"event.kind": "security_review_due", "security.pentest_age": 0.0, "security.vulnerability_assessment_age": 0.0, "security.internal_audit_age": 182*day + 1}, want: fired, branch: "Internal audit due"},
		{name: "audit finding opened", kinds: []string{"audit_finding_opened"}, facts: F{"event.kind": "audit_finding_opened"}, want: fired, branch: "Audit finding"},
		{name: "test dates not recorded", kinds: []string{"security_review_due"}, facts: F{"event.kind": "security_review_due"}, want: notEv},
	},
	// NOT (certificate OR transfer authorisation OR consent) OR no written contract.
	"RW-SEC-07": {
		{name: "no authority of any kind", facts: F{"data.personal": true, "data.destination_foreign": true,
			"data.ncsa_certificate_authorises": false, "data.ncsa_transfer_authorised": false, "data.consent_recorded": false, "data.contract_written": true}, want: fired},
		{name: "consent alone is enough", facts: F{"data.personal": true, "data.destination_foreign": true,
			"data.ncsa_certificate_authorises": false, "data.ncsa_transfer_authorised": false, "data.consent_recorded": true, "data.contract_written": true}, want: clr},
		{name: "authorised but no written contract", facts: F{"data.personal": true, "data.destination_foreign": true,
			"data.ncsa_certificate_authorises": true, "data.contract_written": false}, want: fired},
		{name: "contract not recorded", facts: F{"data.personal": true, "data.destination_foreign": true, "data.ncsa_certificate_authorises": true}, want: notEv},
		{name: "stays in Rwanda", facts: F{"data.personal": true, "data.destination_foreign": false}, want: notApp},
	},
	"RW-SEC-08": {
		{name: "unencrypted store", facts: F{"datastore.encrypted_at_rest": false}, want: fired},
		{name: "encrypted, TLS enforced", facts: F{"datastore.encrypted_at_rest": true, "link.tls_enforced": true}, want: clr},
		{name: "TLS not scanned", facts: F{"datastore.encrypted_at_rest": true}, want: notEv},
	},
	"RW-SEC-09": {
		{name: "assessment a second over a year old", facts: F{"vendor.due_diligence_done": true, "vendor.assessment_age": 365*day + 1}, want: fired},
		{name: "assessed a year ago", facts: F{"vendor.due_diligence_done": true, "vendor.assessment_age": 365 * day}, want: clr},
		{name: "no due diligence", facts: F{"vendor.due_diligence_done": false}, want: fired},
		{name: "assessment date missing", facts: F{"vendor.due_diligence_done": true}, want: notEv},
	},
	"RW-SEC-15": {
		{name: "illegal computer use", facts: F{"incident.kind": "illegal_computer_use"}, want: fired},
		{name: "another incident", facts: F{"incident.kind": "fraud"}, want: clr},
		{name: "not classified", want: notEv},
	},
	"RW-SEC-16": {
		{name: "illegal SIM swap", facts: F{"incident.kind": "illegal_sim_swap"}, want: fired},
		{name: "another incident", facts: F{"incident.kind": "fraud"}, want: clr},
		{name: "not classified", want: notEv},
	},
	// The comparisons that only choose between branches (branchOnly in countries_test.go).
	"RW-SEC-01": {
		{name: "test by an uncertified tester", kinds: []string{"pentest_completed"},
			facts: F{"event.kind": "pentest_completed", "pentest.tester_certified": false}, want: fired, branch: "Test by an uncertified tester"},
		{name: "test by a certified tester", kinds: []string{"pentest_completed"},
			facts: F{"event.kind": "pentest_completed", "pentest.tester_certified": true}, want: fired, branch: "Test completed"},
		{name: "certification not recorded", kinds: []string{"pentest_completed"},
			facts: F{"event.kind": "pentest_completed"}, want: fired, branch: "Test completed"},
	},
	"RW-SEC-04": {
		{name: "high risk to the people affected", facts: F{"breach.personal_data": true, "breach.high_risk": true}, want: fired, branch: "High risk to the people affected"},
		{name: "personal data, not high risk", facts: F{"breach.personal_data": true, "breach.high_risk": false}, want: fired, branch: "Personal-data breach"},
		{name: "risk not assessed", facts: F{"breach.personal_data": true}, want: fired, branch: "Personal-data breach"},
		{name: "no personal data", facts: F{"breach.personal_data": false}, want: clr},
	},
}

func TestCountryRulesFromTheLibrary(t *testing.T) {
	defs := map[string]*pack.Rule{}
	for _, country := range []string{"GH", "RW"} {
		p := countryPack(t, country)
		for i := range p.Rules {
			defs[p.Rules[i].ID] = &p.Rules[i]
		}
	}
	for id, list := range countryCases {
		def, ok := defs[id]
		require.True(t, ok, "%s is not in a pack", id)
		for _, c := range list {
			t.Run(id+"/"+c.name, func(t *testing.T) {
				kinds := c.kinds
				if kinds == nil {
					kinds = def.Trigger.Events[:1]
				}
				aggs := c.aggs
				if aggs == nil {
					aggs = A{}
				}
				res := interp.Evaluate(interp.Compile(def, c.params), interp.Env{Kinds: kinds, Facts: c.facts, Aggs: aggs})
				assert.Equal(t, c.want, res.Status, "missing %v", res.Missing)
				if c.missing != "" {
					assert.Contains(t, res.Missing, c.missing)
				}
				if c.weight != 0 {
					assert.Equal(t, c.weight, res.Weight)
				}
				if c.branch != "" {
					assert.Equal(t, c.branch, res.Branch)
				}
				assert.Equal(t, c.exemptionUnknown, res.ExemptionUnknown)
			})
		}
	}
	// Every regulatory control of both countries has cases from the library.
	for id, def := range defs {
		if def.InheritedFrom == "" && def.Status == pack.StatusRegulatory && def.Kind != pack.KindGovernance {
			_, ok := countryCases[id]
			assert.True(t, ok, "%s has no cases from the library", id)
		}
	}
}

// The "just under the limit" bands (GH-TM-07: 90% to 99.9% of a wallet's daily limit) count a
// payment at 90% and at 99.9% of the limit, and not at 89.99% or 99.91%.
func TestJustUnderTheLimitBands(t *testing.T) {
	p := countryPack(t, "GH")
	var def *pack.Rule
	for i := range p.Rules {
		if p.Rules[i].ID == "GH-TM-07" {
			def = &p.Rules[i]
		}
	}
	require.NotNil(t, def)
	r := interp.Compile(def, nil)
	limits := map[float64]bool{} // daily limits in pesewas whose band was found
	for _, ref := range r.Aggregates() {
		if ref.Fn != "count" {
			continue
		}
		for _, limit := range []float64{300_000, 1_500_000, 2_500_000} {
			at := func(share float64) interp.Tri {
				return interp.Where(ref, interp.Facts{"txn.direction": "debit", "txn.amount": interp.Money{Minor: limit * share, Currency: "GHS"}})
			}
			if at(0.9) != interp.True {
				continue
			}
			limits[limit] = true
			assert.Equal(t, interp.False, at(0.8999), "89.99%% of %v", limit)
			assert.Equal(t, interp.True, at(0.999), "99.9%% of %v", limit)
			assert.Equal(t, interp.False, at(0.9991), "99.91%% of %v", limit)
			assert.Equal(t, interp.False, interp.Where(ref, interp.Facts{"txn.direction": "credit", "txn.amount": interp.Money{Minor: limit * 0.95, Currency: "GHS"}}),
				"a credit is not a payment out")
		}
	}
	assert.Len(t, limits, 3, "a band for each tier's limit")
	// A heavy day is 80% of the tier's daily limit.
	var over []float64
	for _, ref := range r.Aggregates() {
		if ref.Fn == "days_over" {
			assert.Equal(t, "GHS", ref.OverCurrency)
			assert.Equal(t, int64(604800), ref.Window)
			over = append(over, ref.Over)
		}
	}
	assert.ElementsMatch(t, []float64{240_000, 1_200_000, 2_000_000}, over)
}
