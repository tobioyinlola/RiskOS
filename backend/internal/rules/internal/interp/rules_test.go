package interp_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
)

// ngn is an amount in naira, as minor units (kobo).
func ngn(naira float64) interp.Money { return interp.Money{Minor: naira * 100, Currency: "NGN"} }

// kobo is an exact amount in minor units.
func kobo(minor float64) interp.Money { return interp.Money{Minor: minor, Currency: "NGN"} }

type F = interp.Facts

// A is aggregate values by "fn(event[.field]) by entity", optionally suffixed "@window".
type A map[string]any

func (a A) Value(ref interp.AggRef) (any, bool) {
	k := ref.Fn + "(" + ref.Event
	if ref.Field != "" {
		k += "." + ref.Field
	}
	k += ") by " + ref.By
	if v, ok := a[fmt.Sprintf("%s@%d", k, ref.Window)]; ok {
		return v, true
	}
	v, ok := a[k]
	return v, ok
}

// tc is one case: an event of kinds (the rule's first trigger by default) with facts, aggregates
// and settings, and what the rule must do.
type tc struct {
	name   string
	kinds  []string
	facts  F
	aggs   A
	params map[string]float64
	want   interp.Status
	// missing, when set, must be among the result's missing items.
	missing string
	weight  int
	branch  string
	fee     float64
	hold    int64
	// exemptionUnknown expects a control that fired without its exemption being checked.
	exemptionUnknown bool
}

const (
	fired  = interp.StatusFired
	clr    = interp.StatusClear
	notEv  = interp.StatusNotEvaluated
	notApp = interp.StatusNotApplicable
	day    = 86400.0
)

// cases are written from the library's plain wording (docs/compliance/NIGERIA_RULES_LIBRARY.md
// section 7) at each threshold: the first value that fires, the last that does not, and missing
// data.
var cases = map[string][]tc{ //nolint:gochecknoglobals // test table
	"ATO-01": {
		{name: "new device and new payee, then a large transfer", params: map[string]float64{"floor": 10_000_000},
			facts: F{"txn.amount": kobo(10_000_001)}, aggs: A{"exists(device_bound) by account": true, "exists(beneficiary_added) by account": true}, want: fired, weight: 30},
		{name: "at the floor", params: map[string]float64{"floor": 10_000_000},
			facts: F{"txn.amount": kobo(10_000_000)}, aggs: A{"exists(device_bound) by account": true, "exists(beneficiary_added) by account": true}, want: clr},
		{name: "no new device", params: map[string]float64{"floor": 10_000_000},
			facts: F{"txn.amount": kobo(10_000_001)}, aggs: A{"exists(device_bound) by account": false, "exists(beneficiary_added) by account": true}, want: clr},
		{name: "the floor is the institution's to set", facts: F{"txn.amount": ngn(1_000_000)},
			aggs: A{"exists(device_bound) by account": true, "exists(beneficiary_added) by account": true}, want: notEv, missing: "setting:floor"},
	},
	"ATO-02": {
		{name: "credential change then a large transfer", params: map[string]float64{"floor": 5_000_000},
			facts: F{"txn.amount": kobo(5_000_001)}, aggs: A{"exists(profile_change) by account": true}, want: fired, weight: 25},
		{name: "at the floor", params: map[string]float64{"floor": 5_000_000},
			facts: F{"txn.amount": kobo(5_000_000)}, aggs: A{"exists(profile_change) by account": true}, want: clr},
		{name: "history unavailable", params: map[string]float64{"floor": 5_000_000},
			facts: F{"txn.amount": kobo(5_000_001)}, want: notEv, missing: "history:exists profile_change by account"},
	},
	"ATO-03": {
		{name: "SIM swap reported", facts: F{"telco.sim_swap": true}, want: fired, weight: 25},
		{name: "no swap, no re-registration", facts: F{"telco.sim_swap": false},
			aggs: A{"exists(sim_reregistered) by account": false, "exists(device_reregistered) by account": false}, want: clr},
		{name: "re-registration without telco data", aggs: A{"exists(sim_reregistered) by account": true}, want: fired},
		{name: "no telco data and nothing else", aggs: A{"exists(sim_reregistered) by account": false, "exists(device_reregistered) by account": false},
			want: notEv, missing: "fact:telco.sim_swap"},
	},
	"ATO-04": {
		{name: "rooted", facts: F{"device.rooted": true}, want: fired, weight: 20},
		{name: "clean device", facts: F{"device.rooted": false, "device.jailbroken": false, "device.emulator": false, "device.hooking_framework": false}, want: clr},
		{name: "integrity not reported", want: notEv, missing: "fact:device.rooted"},
	},
	"ATO-05": {
		{name: "two sessions", facts: F{"session.open_count": 2.0, "session.ip_changed": false, "session.user_agent_changed": false, "session.device_changed": false}, want: fired, weight: 20},
		{name: "one session, nothing changed", facts: F{"session.open_count": 1.0, "session.ip_changed": false, "session.user_agent_changed": false, "session.device_changed": false}, want: clr},
		{name: "session count unknown", facts: F{"session.ip_changed": false, "session.user_agent_changed": false, "session.device_changed": false}, want: notEv, missing: "fact:session.open_count"},
	},
	"ATO-06": {
		{name: "three failed codes then success", aggs: A{"count(auth_failure) by account": 3.0}, want: fired, weight: 15},
		{name: "two failures", aggs: A{"count(auth_failure) by account": 2.0}, want: clr},
		{name: "history unavailable", want: notEv},
	},
	"BEH-01": {
		{name: "above three deviations", facts: F{"history.amount_zscore": 3.01, "history.amount_to_max": 1.0, "customer.history_age": 30 * day}, want: fired, weight: 24},
		{name: "five times the largest", facts: F{"history.amount_zscore": 0.0, "history.amount_to_max": 5.01, "customer.history_age": 30 * day}, want: fired},
		{name: "at the thresholds", facts: F{"history.amount_zscore": 3.0, "history.amount_to_max": 5.0, "customer.history_age": 30 * day}, want: clr},
		{name: "too little history", facts: F{"history.amount_zscore": 10.0, "customer.history_age": 30*day - 1}, want: clr},
		{name: "history age unknown", facts: F{"history.amount_zscore": 10.0}, want: notEv, missing: "fact:customer.history_age"},
	},
	"BEH-02": {
		{name: "impossible travel outranks a new country", facts: F{"geo.country_usual": false, "geo.speed_from_previous": 901.0}, want: fired, weight: 20, branch: "Impossible travel"},
		{name: "new country only", facts: F{"geo.country_usual": false, "geo.speed_from_previous": 900.0}, want: fired, weight: 10, branch: "New country or region"},
		{name: "usual place, possible speed", facts: F{"geo.country_usual": true, "geo.speed_from_previous": 900.0}, want: clr},
		{name: "no location history", facts: F{"geo.country_usual": true}, want: notEv, missing: "fact:geo.speed_from_previous"},
	},
	"BEH-03": {
		{name: "unusual hour", facts: F{"history.hour_usual": false, "customer.history_age": 30 * day}, want: fired, weight: 8},
		{name: "usual hour", facts: F{"history.hour_usual": true, "customer.history_age": 30 * day}, want: clr},
		{name: "hour pattern unknown", facts: F{"customer.history_age": 30 * day}, want: notEv, missing: "fact:history.hour_usual"},
	},
	"CNP-01": {
		{name: "ten small or declined attempts on a card", aggs: A{"count(card_authorization) by card": 10.0, "count(card_authorization) by ip": 0.0, "count(card_authorization) by device": 0.0}, want: fired, weight: 25},
		{name: "nine", aggs: A{"count(card_authorization) by card": 9.0, "count(card_authorization) by ip": 9.0, "count(card_authorization) by device": 9.0}, want: clr},
		{name: "card window unavailable", aggs: A{"count(card_authorization) by ip": 0.0, "count(card_authorization) by device": 0.0}, want: notEv, missing: "history:count card_authorization by card"},
	},
	"CNP-02": {
		{name: "five cards on one device", aggs: A{"distinct_count(card_authorization.card.id) by device": 5.0, "distinct_count(card_authorization.card.id) by ip": 1.0}, want: fired, weight: 25},
		{name: "four", aggs: A{"distinct_count(card_authorization.card.id) by device": 4.0, "distinct_count(card_authorization.card.id) by ip": 4.0}, want: clr},
		{name: "unavailable", want: notEv},
	},
	"CNP-03": {
		{name: "refunds above three times normal", kinds: []string{"merchant_daily"}, facts: F{"merchant.refund_rate_to_baseline": 3.01}, want: fired, weight: 20},
		{name: "at three times", kinds: []string{"merchant_daily"}, facts: F{"merchant.refund_rate_to_baseline": 3.0, "merchant.chargeback_rate_to_baseline": 3.0, "terminal.impossible_location": false}, want: clr},
		{name: "no merchant monitoring", kinds: []string{"merchant_daily"}, want: notEv},
	},
	"CRY-01": {
		{name: "80% of purchases sent out", aggs: A{"exists(fiat_inflow) by account": true, "exists(crypto_purchase) by account": true,
			"sum(crypto_withdrawal.txn.amount) by account": ngn(800), "sum(crypto_purchase.txn.amount) by account": ngn(1000)}, want: fired, weight: 25},
		{name: "just under 80%", aggs: A{"exists(fiat_inflow) by account": true, "exists(crypto_purchase) by account": true,
			"sum(crypto_withdrawal.txn.amount) by account": kobo(79_999), "sum(crypto_purchase.txn.amount) by account": ngn(1000)}, want: clr},
		{name: "inflow unknown", aggs: A{"exists(crypto_purchase) by account": true,
			"sum(crypto_withdrawal.txn.amount) by account": ngn(800), "sum(crypto_purchase.txn.amount) by account": ngn(1000)}, want: notEv},
	},
	"CRY-02": {
		{name: "five funding sources, 80% out", aggs: A{"distinct_count(funding.funding_source.id) by account": 5.0,
			"sum(transfer_outbound.txn.amount) by account": ngn(800), "sum(transfer_inbound.txn.amount) by account": ngn(1000)}, want: fired, weight: 25},
		{name: "four sources", aggs: A{"distinct_count(funding.funding_source.id) by account": 4.0,
			"sum(transfer_outbound.txn.amount) by account": ngn(800), "sum(transfer_inbound.txn.amount) by account": ngn(1000)}, want: clr},
		{name: "inflows unknown", aggs: A{"distinct_count(funding.funding_source.id) by account": 5.0, "sum(transfer_outbound.txn.amount) by account": ngn(800)}, want: notEv},
	},
	"CRY-03": {
		{name: "mixer", facts: F{"analytics.wallet_tag": "mixer"}, want: fired, weight: 35},
		{name: "exchange", facts: F{"analytics.wallet_tag": "exchange"}, want: clr},
		{name: "no analytics", want: notEv, missing: "fact:analytics.wallet_tag"},
	},
	"CRY-04": {
		{name: "above the threshold", params: map[string]float64{"va_threshold": 100_000_000}, facts: F{"txn.amount": kobo(100_000_001)}, want: fired, weight: 15},
		{name: "at the threshold", params: map[string]float64{"va_threshold": 100_000_000}, facts: F{"txn.amount": kobo(100_000_000)}, want: clr},
		{name: "awaiting the SEC threshold", facts: F{"txn.amount": ngn(1)}, want: notEv, missing: "setting:va_threshold"},
	},
	"INS-01": {
		{name: "employee-linked account above peers", facts: F{"account.linked_to_employee": true, "account.volume_above_peer_p95": true}, want: fired, weight: 20},
		{name: "employee-linked, normal", facts: F{"account.linked_to_employee": true, "account.volume_above_peer_p95": false, "counterparty.serviced_by_linked_employee": false}, want: clr},
		{name: "not linked to an employee", facts: F{"account.linked_to_employee": false}, want: notApp},
		{name: "staff directory missing", want: notEv, missing: "fact:account.linked_to_employee"},
	},
	"INS-02": {
		{name: "staff viewed contact details before the transfer", aggs: A{"exists(staff_action) by customer": true}, want: fired, weight: 30},
		{name: "no staff action", aggs: A{"exists(staff_action) by customer": false}, want: clr},
		{name: "unavailable", want: notEv},
	},
	"LEN-01": {
		{name: "three applications per BVN", aggs: A{"count(loan_application) by bvn": 3.0, "count(loan_application) by device": 1.0}, want: fired, weight: 20},
		{name: "two", aggs: A{"count(loan_application) by bvn": 2.0, "count(loan_application) by device": 2.0}, want: clr},
		{name: "no BVN window", aggs: A{"count(loan_application) by device": 1.0}, want: notEv, missing: "history:count loan_application by bvn"},
	},
	"LEN-02": {
		{name: "90% of a loan out quickly", aggs: A{"exists(loan_disbursement) by account": true,
			"sum(transfer_outbound.txn.amount) by account": ngn(900), "sum(loan_disbursement.txn.amount) by account": ngn(1000)}, want: fired, weight: 20},
		{name: "just under 90%", aggs: A{"exists(loan_disbursement) by account": true,
			"sum(transfer_outbound.txn.amount) by account": kobo(89_999), "sum(loan_disbursement.txn.amount) by account": ngn(1000)}, want: clr},
		{name: "unavailable", want: notEv},
	},
	"LEN-03": {
		{name: "photo reused", facts: F{"application.photo_reused": true}, want: fired, weight: 25},
		{name: "nothing reused", facts: F{"application.photo_reused": false, "application.document_reused": false, "application.phone_reused": false}, want: clr},
		{name: "no identity matching", want: notEv},
	},
	"LEN-04": {
		{name: "three repayments from others", aggs: A{"count(loan_repayment) by loan": 3.0}, want: fired, weight: 15},
		{name: "two", aggs: A{"count(loan_repayment) by loan": 2.0}, want: clr},
		{name: "unavailable", want: notEv},
	},
	"MUL-01": {
		{name: "80% out after a large inflow", params: map[string]float64{"floor": 10_000_000},
			aggs: A{"sum(transfer_outbound.txn.amount) by account": kobo(8_000_001), "sum(transfer_inbound.txn.amount) by account": kobo(10_000_001)}, want: fired, weight: 25},
		{name: "inflow at the floor", params: map[string]float64{"floor": 10_000_000},
			aggs: A{"sum(transfer_outbound.txn.amount) by account": kobo(9_000_000), "sum(transfer_inbound.txn.amount) by account": kobo(10_000_000)}, want: clr},
		{name: "little out, floor unset: clear without the floor", aggs: A{"sum(transfer_outbound.txn.amount) by account": ngn(1), "sum(transfer_inbound.txn.amount) by account": ngn(1000)}, want: clr},
		{name: "80% out, floor unset", aggs: A{"sum(transfer_outbound.txn.amount) by account": ngn(900), "sum(transfer_inbound.txn.amount) by account": ngn(1000)}, want: notEv, missing: "setting:floor"},
	},
	"MUL-02": {
		{name: "ten senders", kinds: []string{"transfer_inbound"}, aggs: A{"distinct_count(transfer_inbound.sender.id) by account": 10.0}, want: fired, weight: 20},
		{name: "nine", kinds: []string{"transfer_inbound"}, aggs: A{"distinct_count(transfer_inbound.sender.id) by account": 9.0}, want: clr},
		{name: "unavailable", kinds: []string{"transfer_inbound"}, want: notEv},
	},
	"MUL-03": {
		{name: "five recipients from one inflow", aggs: A{"distinct_count(transfer_outbound.beneficiary.id) by inflow": 5.0}, want: fired, weight: 20},
		{name: "four", aggs: A{"distinct_count(transfer_outbound.beneficiary.id) by inflow": 4.0}, want: clr},
		{name: "the inflow is not identified", want: notEv, missing: "history:distinct_count transfer_outbound by inflow"},
	},
	"MUL-04": {
		{name: "new account above ten times its tier", facts: F{"account.age": 3600.0, "account.kyc_tier_norm": ngn(1000)},
			aggs: A{"sum(transaction_any.txn.amount) by account": kobo(1_000_001)}, want: fired, weight: 20},
		{name: "at ten times", facts: F{"account.age": 3600.0, "account.kyc_tier_norm": ngn(1000)},
			aggs: A{"sum(transaction_any.txn.amount) by account": ngn(10_000)}, want: clr},
		{name: "older account", facts: F{"account.age": 7 * day}, want: notApp},
		{name: "tier unknown", facts: F{"account.age": 3600.0}, aggs: A{"sum(transaction_any.txn.amount) by account": ngn(10_000)}, want: notEv, missing: "fact:account.kyc_tier_norm"},
	},
	"MUL-05": {
		{name: "dormant 90 days then large", params: map[string]float64{"large": 50_000_000}, facts: F{"account.inactive_for": 90 * day, "txn.amount": kobo(50_000_001)}, want: fired, weight: 20},
		{name: "a day short of dormant", params: map[string]float64{"large": 50_000_000}, facts: F{"account.inactive_for": 89 * day, "txn.amount": kobo(50_000_001)}, want: clr},
		{name: "large amount unset", facts: F{"account.inactive_for": 90 * day, "txn.amount": ngn(1)}, want: notEv, missing: "setting:large"},
	},
	"MUL-06": {
		{name: "three accounts on a device", aggs: A{"distinct_count(activity_any.account.id) by device": 3.0, "distinct_count(activity_any.account.id) by bvn": 1.0,
			"distinct_count(activity_any.account.id) by phone": 1.0, "distinct_count(activity_any.account.id) by address": 1.0}, want: fired, weight: 25},
		{name: "two accounts per BVN", aggs: A{"distinct_count(activity_any.account.id) by device": 1.0, "distinct_count(activity_any.account.id) by bvn": 2.0,
			"distinct_count(activity_any.account.id) by phone": 1.0, "distinct_count(activity_any.account.id) by address": 1.0}, want: fired},
		{name: "below both", aggs: A{"distinct_count(activity_any.account.id) by device": 2.0, "distinct_count(activity_any.account.id) by bvn": 1.0,
			"distinct_count(activity_any.account.id) by phone": 1.0, "distinct_count(activity_any.account.id) by address": 1.0}, want: clr},
		{name: "identity windows unavailable", aggs: A{"distinct_count(activity_any.account.id) by device": 1.0}, want: notEv},
	},
	"MUL-07": {
		{name: "three reporters", aggs: A{"distinct_count(fraud_report.reporter.id) by beneficiary": 3.0}, want: fired, weight: 30},
		{name: "on the fraud list", facts: F{"beneficiary.on_fraud_list": true}, want: fired},
		{name: "two reporters, on no list", facts: F{"beneficiary.on_fraud_list": false, "beneficiary.on_poi_list": false},
			aggs: A{"distinct_count(fraud_report.reporter.id) by beneficiary": 2.0}, want: clr},
		{name: "lists unavailable", aggs: A{"distinct_count(fraud_report.reporter.id) by beneficiary": 2.0}, want: notEv},
	},
	"MUL-08": {
		{name: "70% of a card inflow cashed out", facts: F{"txn.amount": ngn(700)},
			aggs: A{"exists(transfer_inbound) by account": true, "sum(transfer_inbound.txn.amount) by account": ngn(1000)}, want: fired, weight: 15},
		{name: "just under 70%", facts: F{"txn.amount": kobo(69_999)},
			aggs: A{"exists(transfer_inbound) by account": true, "sum(transfer_inbound.txn.amount) by account": ngn(1000)}, want: clr},
		{name: "unavailable", facts: F{"txn.amount": ngn(700)}, want: notEv},
	},
	"PON-01": {
		{name: "twenty unrelated senders paid by earlier money", kinds: []string{"transfer_inbound"}, facts: F{"flows.prior_sender_outflow_share": 0.3},
			aggs: A{"distinct_count(transfer_inbound.sender.id) by account": 20.0}, want: fired, weight: 25},
		{name: "nineteen", kinds: []string{"transfer_inbound"}, facts: F{"flows.prior_sender_outflow_share": 0.9},
			aggs: A{"distinct_count(transfer_inbound.sender.id) by account": 19.0}, want: clr},
		{name: "flows unknown", kinds: []string{"transfer_inbound"}, aggs: A{"distinct_count(transfer_inbound.sender.id) by account": 20.0}, want: notEv, missing: "fact:flows.prior_sender_outflow_share"},
	},
	"REG-01": {
		{name: "₦20,000.01 within 24 hours of a new device", facts: F{"txn.amount": kobo(2_000_001), "device.bound_age": 3600.0, "account.age": 400 * day}, want: fired},
		{name: "exactly ₦20,000", facts: F{"txn.amount": ngn(20_000), "device.bound_age": 3600.0, "account.age": 400 * day}, want: clr},
		{name: "device bound 24 hours ago", facts: F{"txn.amount": ngn(50_000), "device.bound_age": day, "account.age": 400 * day}, want: clr},
		{name: "new account", facts: F{"txn.amount": ngn(50_000), "device.bound_age": 30 * day, "account.age": 3600.0}, want: fired},
		{name: "binding status unknown", facts: F{"txn.amount": ngn(50_000), "account.age": 400 * day}, want: notEv, missing: "fact:device.bound_age"},
		{name: "a dollar amount is not compared with a naira limit", facts: F{"txn.amount": interp.Money{Minor: 9_000_000, Currency: "USD"}, "device.bound_age": 3600.0}, want: notEv, missing: "currency:txn.amount"},
	},
	"REG-02": {
		{name: "app login from an unbound device", facts: F{"event.channel": "mobile_app", "account.has_bound_device": true, "device.is_bound_device": false}, kinds: []string{"login"}, want: fired},
		{name: "the bound device", facts: F{"event.channel": "mobile_app", "account.has_bound_device": true, "device.is_bound_device": true}, kinds: []string{"login"}, want: clr},
		{name: "web login", facts: F{"event.channel": "web"}, kinds: []string{"login"}, want: notApp},
		{name: "binding unknown", facts: F{"event.channel": "mobile_app"}, kinds: []string{"login"}, want: notEv},
	},
	"REG-03": {
		{name: "web login from a new device", facts: F{"event.channel": "web", "device.is_new": true}, kinds: []string{"login"}, want: fired},
		{name: "known device", facts: F{"event.channel": "web", "device.is_new": false}, kinds: []string{"login"}, want: clr},
		{name: "channel unknown", kinds: []string{"login"}, want: notEv, missing: "fact:event.channel"},
	},
	"REG-04": {
		{name: "individual above ₦25 million", facts: F{"customer.type": "individual", "txn.amount": kobo(2_500_000_001), "account.limit_raised_with_edd_and_mfa": false}, want: fired},
		{name: "at ₦25 million", facts: F{"customer.type": "individual", "txn.amount": ngn(25_000_000), "account.limit_raised_with_edd_and_mfa": false}, want: clr},
		{name: "limit raised after EDD and MFA", facts: F{"customer.type": "individual", "txn.amount": ngn(30_000_000), "account.limit_raised_with_edd_and_mfa": true}, want: clr},
		{name: "limit status unknown: the legal limit applies", facts: F{"customer.type": "individual", "txn.amount": ngn(30_000_000)}, want: fired, exemptionUnknown: true},
		{name: "company above ₦250 million", facts: F{"customer.type": "corporate", "txn.amount": kobo(25_000_000_001), "account.limit_raised_with_edd_and_mfa": false}, want: fired},
		{name: "customer type unknown", facts: F{"txn.amount": ngn(30_000_000)}, want: notEv, missing: "fact:customer.type"},
	},
	"REG-05": {
		{name: "BVN on the suspect list", facts: F{"customer.bvn_on_suspect_list": true}, want: fired, hold: 86400},
		{name: "not listed", facts: F{"customer.bvn_on_suspect_list": false}, want: clr},
		{name: "list not connected", want: notEv, missing: "fact:customer.bvn_on_suspect_list"},
	},
	"REG-06": {
		{name: "liveness failed", kinds: []string{"account_opened"}, facts: F{"event.channel": "digital", "kyc.liveness_passed": false, "kyc.bvn_live_check": "verified", "kyc.nin_live_check": "verified"}, want: fired},
		{name: "BVN not checked live", kinds: []string{"account_opened"}, facts: F{"event.channel": "digital", "kyc.liveness_passed": true, "kyc.bvn_live_check": "not_done", "kyc.nin_live_check": "verified"}, want: fired},
		{name: "all verified", kinds: []string{"account_opened"}, facts: F{"event.channel": "digital", "kyc.liveness_passed": true, "kyc.bvn_live_check": "verified", "kyc.nin_live_check": "verified"}, want: clr},
		{name: "branch onboarding", kinds: []string{"account_opened"}, facts: F{"event.channel": "branch"}, want: notApp},
		{name: "checks unknown", kinds: []string{"account_opened"}, facts: F{"event.channel": "digital", "kyc.liveness_passed": true}, want: notEv},
	},
	"REG-07": {
		{name: "individual cash above ₦5 million", kinds: []string{"cash_payment"}, facts: F{"customer.type": "individual", "txn.amount": kobo(500_000_001)}, want: fired},
		{name: "at ₦5 million", kinds: []string{"cash_payment"}, facts: F{"customer.type": "individual", "txn.amount": ngn(5_000_000)}, want: clr},
		{name: "type unknown", kinds: []string{"cash_payment"}, facts: F{"txn.amount": ngn(6_000_000)}, want: notEv},
	},
	"REG-08": {
		{name: "individual above ₦1.5 million a week: 3%", facts: F{"customer.type": "individual"}, aggs: A{"sum(cash_withdrawal.txn.amount) by customer": kobo(150_000_001)}, want: fired, fee: 3, branch: "Individual"},
		{name: "company above ₦5 million a week: 5%", facts: F{"customer.type": "corporate"}, aggs: A{"sum(cash_withdrawal.txn.amount) by customer": kobo(500_000_001)}, want: fired, fee: 5, branch: "Company"},
		{name: "individual at ₦1.5 million", facts: F{"customer.type": "individual"}, aggs: A{"sum(cash_withdrawal.txn.amount) by customer": ngn(1_500_000)}, want: clr},
		{name: "type unknown", aggs: A{"sum(cash_withdrawal.txn.amount) by customer": ngn(2_000_000)}, want: notEv},
	},
	"REG-09": {
		{name: "above ₦100,000 a day", aggs: A{"sum(atm_withdrawal.txn.amount) by customer@86400": kobo(10_000_001), "sum(atm_withdrawal.txn.amount) by customer@604800": kobo(10_000_001)}, want: fired},
		{name: "above ₦850,000 a week only", aggs: A{"sum(atm_withdrawal.txn.amount) by customer@86400": ngn(100_000), "sum(atm_withdrawal.txn.amount) by customer@604800": kobo(85_000_001)}, want: fired},
		{name: "at both limits", aggs: A{"sum(atm_withdrawal.txn.amount) by customer@86400": ngn(100_000), "sum(atm_withdrawal.txn.amount) by customer@604800": ngn(850_000)}, want: clr},
		{name: "unavailable", want: notEv},
	},
	"REG-10": {
		{name: "$10,000 abroad", facts: F{"txn.amount_usd": interp.Money{Minor: 1_000_000, Currency: "USD"}}, want: fired},
		{name: "$9,999.99", facts: F{"txn.amount_usd": interp.Money{Minor: 999_999, Currency: "USD"}}, want: clr},
		{name: "no exchange rate", facts: F{"txn.amount": ngn(20_000_000)}, want: notEv, missing: "fact:txn.amount_usd"},
	},
	"REG-11": {
		{name: "confirmed sanctions match", facts: F{"screening.sanctions_score": 90.0, "screening.sanctions_confirmed": true}, want: fired},
		{name: "strong match, not confirmed, auto-block off", facts: F{"screening.sanctions_score": 95.0, "screening.sanctions_confirmed": false}, want: clr},
		{name: "strong match, auto-block on", params: map[string]float64{"auto_block_strong_match": 1}, facts: F{"screening.sanctions_score": 90.0, "screening.sanctions_confirmed": false}, want: fired},
		{name: "score 89", facts: F{"screening.sanctions_score": 89.0, "screening.sanctions_confirmed": true}, want: clr},
		{name: "not screened", want: notEv, missing: "fact:screening.sanctions_score"},
	},
	"REG-12": {
		{name: "PEP", facts: F{"customer.is_pep": true}, want: fired},
		{name: "not a PEP, no PEP owner", facts: F{"customer.is_pep": false, "customer.has_pep_owner": false}, want: clr},
		{name: "PEP list not connected", want: notEv},
	},
	"REG-13": {
		{name: "person of interest", facts: F{"party.on_poi_list": true}, want: fired},
		{name: "not listed", facts: F{"party.on_poi_list": false}, want: clr},
		{name: "list not connected", want: notEv},
	},
	"REG-14": {
		{name: "suspicion confirmed starts the STR clock", kinds: []string{"suspicion_confirmed"}, want: fired},
		{name: "a transfer is not a workflow event", kinds: []string{"transfer_outbound"}, want: notApp},
	},
	"REG-15": {
		{name: "active watchlist entry", kinds: []string{"login"}, facts: F{"bvn_watchlist.category": "1", "bvn_watchlist.active": true}, want: fired},
		{name: "lifted entry", kinds: []string{"login"}, facts: F{"bvn_watchlist.category": "1", "bvn_watchlist.active": false}, want: clr},
		{name: "watchlist not connected", kinds: []string{"login"}, want: notEv},
	},
	"REG-16": {
		{name: "no BVN", kinds: []string{"account_opened"}, facts: F{"account.has_bvn": false}, want: fired},
		{name: "BVN on file", kinds: []string{"account_opened"}, facts: F{"account.has_bvn": true}, want: clr},
		{name: "profile unknown", kinds: []string{"account_opened"}, want: notEv},
	},
	"REG-17": {
		{name: "individual cash of ₦5 million", kinds: []string{"cash_deposit"}, facts: F{"txn.domestic": true, "customer.type": "individual", "txn.amount": ngn(5_000_000)}, want: fired},
		{name: "a kobo under", kinds: []string{"cash_deposit"}, facts: F{"txn.domestic": true, "customer.type": "individual", "txn.amount": kobo(499_999_999)}, want: clr},
		{name: "not domestic", kinds: []string{"cash_deposit"}, facts: F{"txn.domestic": false}, want: notApp},
		{name: "type unknown", kinds: []string{"cash_deposit"}, facts: F{"txn.domestic": true, "txn.amount": ngn(5_000_000)}, want: notEv},
	},
	"REG-18": {
		{name: "PEP transaction", facts: F{"customer.is_pep": true}, want: fired},
		{name: "not a PEP", facts: F{"customer.is_pep": false}, want: clr},
		{name: "unknown", want: notEv},
	},
	"REG-19": {
		{name: "category outside the list", kinds: []string{"report_submit"}, facts: F{"report.has_predicate_offence": true, "report.has_indicator": true, "report.category": "other"}, want: fired},
		{name: "complete report", kinds: []string{"report_submit"}, facts: F{"report.has_predicate_offence": true, "report.has_indicator": true, "report.category": "fraud"}, want: clr},
		{name: "category missing", kinds: []string{"report_submit"}, facts: F{"report.has_predicate_offence": true, "report.has_indicator": true}, want: notEv},
	},
	"SEC-01": {
		{name: "ten failures over five accounts from an address", kinds: []string{"auth_failure"}, aggs: A{"count(auth_failure) by ip": 10.0, "distinct_count(auth_failure.account.id) by ip": 5.0}, want: fired, weight: 30},
		{name: "four accounts", kinds: []string{"auth_failure"}, aggs: A{"count(auth_failure) by ip": 50.0, "distinct_count(auth_failure.account.id) by ip": 4.0}, want: clr},
		{name: "no address", kinds: []string{"auth_failure"}, want: notEv},
	},
	"SEC-02": {
		{name: "fifty accounts tried once", kinds: []string{"auth_failure"}, aggs: A{"distinct_count_once(auth_failure.account.id) by network": 50.0}, want: fired, weight: 25},
		{name: "forty-nine", kinds: []string{"auth_failure"}, aggs: A{"distinct_count_once(auth_failure.account.id) by network": 49.0}, want: clr},
		{name: "unavailable", kinds: []string{"auth_failure"}, want: notEv},
	},
	"SEC-03": {
		{name: "five failures", kinds: []string{"auth_failure"}, aggs: A{"count(auth_failure) by account": 5.0}, want: fired, weight: 20},
		{name: "four", kinds: []string{"auth_failure"}, aggs: A{"count(auth_failure) by account": 4.0}, want: clr},
		{name: "unavailable", kinds: []string{"auth_failure"}, want: notEv},
	},
	"SEC-04": {
		{name: "five prompts", kinds: []string{"mfa_prompt"}, aggs: A{"count(mfa_prompt) by account": 5.0}, want: fired, weight: 25},
		{name: "four", kinds: []string{"mfa_prompt"}, aggs: A{"count(mfa_prompt) by account": 4.0}, want: clr},
		{name: "unavailable", kinds: []string{"mfa_prompt"}, want: notEv},
	},
	"SEC-05": {
		{name: "administrator over Tor", kinds: []string{"login"}, facts: F{"user.is_admin": true, "ip.is_tor": true}, want: fired, weight: 10},
		{name: "administrator, clean address", kinds: []string{"login"}, facts: F{"user.is_admin": true, "ip.is_tor": false, "ip.is_proxy": false, "ip.is_vpn": false}, want: clr},
		{name: "ordinary user, small amount", params: map[string]float64{"high_value": 100_000_000}, facts: F{"user.is_admin": false, "txn.amount": ngn(10)}, want: notApp},
		{name: "ordinary user, high-value unset", facts: F{"user.is_admin": false, "txn.amount": ngn(10)}, want: notEv, missing: "setting:high_value"},
	},
	"SEC-06": {
		{name: "keystrokes too fast", kinds: []string{"session_activity"}, facts: F{"client.headless": false, "input.keystroke_interval_ms": 39.0, "input.timing_variance": 0.5}, want: fired, weight: 20},
		{name: "human pace", kinds: []string{"session_activity"}, facts: F{"client.headless": false, "input.keystroke_interval_ms": 40.0, "input.timing_variance": 0.05}, want: clr},
		{name: "no input telemetry", kinds: []string{"session_activity"}, facts: F{"client.headless": false}, want: notEv},
	},
	"SEC-07": {
		{name: "address and browser changed", kinds: []string{"session_activity"}, facts: F{"session.ip_changed": true, "session.user_agent_changed": true}, want: fired, weight: 25},
		{name: "only the address changed", kinds: []string{"session_activity"}, facts: F{"session.ip_changed": true, "session.user_agent_changed": false, "session.device_changed": false}, want: clr},
		{name: "unknown", kinds: []string{"session_activity"}, facts: F{"session.ip_changed": true}, want: notEv},
	},
	"SEC-08": {
		{name: "key used from an unknown network", kinds: []string{"api_request"}, facts: F{"auth.type": "api_key", "api_key.network_known": false}, want: fired, weight: 30},
		{name: "usual use", kinds: []string{"api_request"}, facts: F{"auth.type": "api_key", "api_key.network_known": true, "api_key.country_known": true, "api_key.rate_to_baseline": 5.0, "api_key.error_ratio": 0.5}, want: clr},
		{name: "a session, not a key", kinds: []string{"api_request"}, facts: F{"auth.type": "session"}, want: notApp},
		{name: "gateway not connected", kinds: []string{"api_request"}, facts: F{"auth.type": "api_key"}, want: notEv},
	},
	"SEC-09": {
		{name: "payout details changed then a large payment", params: map[string]float64{"high_value": 100_000_000}, facts: F{"txn.amount": kobo(100_000_001)}, aggs: A{"exists(config_change) by merchant": true}, want: fired, weight: 30},
		{name: "at the high value", params: map[string]float64{"high_value": 100_000_000}, facts: F{"txn.amount": kobo(100_000_000)}, aggs: A{"exists(config_change) by merchant": true}, want: clr},
		{name: "high value unset", facts: F{"txn.amount": ngn(1)}, aggs: A{"exists(config_change) by merchant": true}, want: notEv, missing: "setting:high_value"},
	},
	"SEC-10": {
		{name: "export soon after a role change", kinds: []string{"data_export"}, aggs: A{"exists(role_change) by user": true}, want: fired, weight: 30},
		{name: "no role change", kinds: []string{"data_export"}, aggs: A{"exists(role_change) by user": false}, want: clr},
		{name: "unavailable", kinds: []string{"data_export"}, want: notEv},
	},
	"SEC-11": {
		{name: "twenty failed lookups", kinds: []string{"api_lookup"}, aggs: A{"count(api_lookup) by ip": 20.0, "count(api_lookup) by api_key": 0.0}, want: fired, weight: 20},
		{name: "nineteen", kinds: []string{"api_lookup"}, aggs: A{"count(api_lookup) by ip": 19.0, "count(api_lookup) by api_key": 19.0}, want: clr},
		{name: "unavailable", kinds: []string{"api_lookup"}, want: notEv},
	},
	"SEC-12": {
		{name: "export above five times the user's normal", kinds: []string{"data_export"}, facts: F{"export.rows_to_user_baseline": 5.01}, want: fired, weight: 25},
		{name: "five times", kinds: []string{"data_export"}, facts: F{"export.rows_to_user_baseline": 5.0, "export.rows_to_tenant_baseline": 5.0}, want: clr},
		{name: "no baseline", kinds: []string{"data_export"}, want: notEv},
	},
	"SIM-01": {
		{name: "phone not linked to the NIN, large transfer", params: map[string]float64{"floor": 5_000_000}, facts: F{"identity.phone_nin_linked": false, "txn.amount": kobo(5_000_001)}, want: fired, weight: 15},
		{name: "linked", params: map[string]float64{"floor": 5_000_000}, facts: F{"identity.phone_nin_linked": true, "txn.amount": kobo(5_000_001)}, want: clr},
		{name: "floor unset", facts: F{"identity.phone_nin_linked": false, "txn.amount": ngn(1)}, want: notEv, missing: "setting:floor"},
	},
	"SIM-02": {
		{name: "second phone change", kinds: []string{"profile_change"}, facts: F{"profile.field": []string{"phone", "email"}}, aggs: A{"count(profile_change) by customer": 2.0}, want: fired, weight: 15},
		{name: "first change", kinds: []string{"profile_change"}, facts: F{"profile.field": []string{"phone"}}, aggs: A{"count(profile_change) by customer": 1.0}, want: clr},
		{name: "an email change", kinds: []string{"profile_change"}, facts: F{"profile.field": []string{"email"}}, want: notApp},
		{name: "unavailable", kinds: []string{"profile_change"}, facts: F{"profile.field": []string{"phone"}}, want: notEv},
	},
	"SIM-03": {
		{name: "subscriber differs", facts: F{"telco.subscriber_matches_profile": false}, want: fired, weight: 25},
		{name: "matches", facts: F{"telco.subscriber_matches_profile": true}, want: clr},
		{name: "no telco data", want: notEv},
	},
	"VEL-01": {
		{name: "five transfers in ten minutes", aggs: A{"count(transfer_outbound) by account": 5.0}, want: fired, weight: 15},
		{name: "four", aggs: A{"count(transfer_outbound) by account": 4.0}, want: clr},
		{name: "velocity unavailable", want: notEv, missing: "history:count transfer_outbound by account"},
	},
	"VEL-02": {
		{name: "a day above three times the daily average", params: map[string]float64{"floor": 10_000_000},
			aggs: A{"sum(transfer_outbound.txn.amount) by account@86400": kobo(30_000_001), "daily_avg(transfer_outbound.txn.amount) by account@2592000": ngn(100_000)}, want: fired, weight: 20},
		{name: "exactly three times", params: map[string]float64{"floor": 10_000_000},
			aggs: A{"sum(transfer_outbound.txn.amount) by account@86400": ngn(300_000), "daily_avg(transfer_outbound.txn.amount) by account@2592000": ngn(100_000)}, want: clr},
		{name: "floor unset", aggs: A{"sum(transfer_outbound.txn.amount) by account@86400": ngn(900_000), "daily_avg(transfer_outbound.txn.amount) by account@2592000": ngn(100_000)}, want: notEv, missing: "setting:floor"},
	},
	"VEL-03": {
		{name: "three new payees in a day", kinds: []string{"beneficiary_added"}, aggs: A{"count(beneficiary_added) by account": 3.0}, want: fired, weight: 15},
		{name: "two", kinds: []string{"beneficiary_added"}, aggs: A{"count(beneficiary_added) by account": 2.0}, want: clr},
		{name: "unavailable", kinds: []string{"beneficiary_added"}, want: notEv},
	},
	"VEL-04": {
		{name: "three payments just under a limit", aggs: A{"count(transaction_any) by account": 3.0}, want: fired, weight: 25},
		{name: "two", aggs: A{"count(transaction_any) by account": 2.0}, want: clr},
		{name: "unavailable", want: notEv},
	},
	"VEL-05": {
		{name: "same round amount to three payees", aggs: A{"max_distinct_per_value(transfer_outbound.beneficiary.id) by account": 3.0}, want: fired, weight: 10},
		{name: "two payees", aggs: A{"max_distinct_per_value(transfer_outbound.beneficiary.id) by account": 2.0}, want: clr},
		{name: "unavailable", want: notEv},
	},
	"VEL-06": {
		{name: "first large payment to a new payee at 4 a.m.", params: map[string]float64{"high_value": 50_000_000},
			facts: F{"beneficiary.is_new": true, "txn.first_to_beneficiary": true, "txn.amount": kobo(50_000_001), "txn.local_hour": 4.0}, want: fired, weight: 15},
		{name: "at 5 a.m.", params: map[string]float64{"high_value": 50_000_000},
			facts: F{"beneficiary.is_new": true, "txn.first_to_beneficiary": true, "txn.amount": kobo(50_000_001), "txn.local_hour": 5.0}, want: clr},
		{name: "high value unset", facts: F{"beneficiary.is_new": true, "txn.first_to_beneficiary": true, "txn.amount": ngn(1), "txn.local_hour": 4.0}, want: notEv, missing: "setting:high_value"},
	},
}

func loadPack(t *testing.T) map[string]*pack.Rule {
	t.Helper()
	p, err := pack.Load(ng.Files)
	require.NoError(t, err)
	out := map[string]*pack.Rule{}
	for i := range p.Rules {
		out[p.Rules[i].ID] = &p.Rules[i]
	}
	return out
}

func TestEveryRuleAtItsBoundaries(t *testing.T) {
	defs := loadPack(t)
	for id, list := range cases {
		def, ok := defs[id]
		require.True(t, ok, "%s is not in the pack", id)
		for _, c := range list {
			t.Run(id+"/"+c.name, func(t *testing.T) {
				r := interp.Compile(def, c.params)
				kinds := c.kinds
				if kinds == nil {
					kinds = def.Trigger.Events[:1]
				}
				aggs := c.aggs
				if aggs == nil {
					aggs = A{}
				}
				res := interp.Evaluate(r, interp.Env{Kinds: kinds, Facts: c.facts, Aggs: aggs})
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
				if c.fee != 0 {
					assert.InDelta(t, c.fee, res.Fee, 1e-9)
				}
				if c.hold != 0 {
					assert.Equal(t, c.hold, res.Hold)
				}
				assert.Equal(t, c.exemptionUnknown, res.ExemptionUnknown)
			})
		}
	}
}

// Every executable rule is tested firing, staying clear and missing data; governance presets
// are workflows (phase 5), and REG-14 is a workflow clock with no condition.
func TestEveryRuleIsCovered(t *testing.T) {
	defs := loadPack(t)
	for id, def := range defs {
		if def.Kind == pack.KindGovernance {
			continue
		}
		var got []interp.Status
		for _, c := range cases[id] {
			got = append(got, c.want)
		}
		want := []interp.Status{fired, clr, notEv}
		if id == "REG-14" {
			want = []interp.Status{fired, notApp}
		}
		for _, s := range want {
			assert.True(t, slices.Contains(got, s), "%s has no %s case", id, s)
		}
	}
}
