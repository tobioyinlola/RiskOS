package pack

// The vocabulary rule definitions may use. Anything not listed here is rejected when a pack is
// loaded, so a definition can only refer to facts the engine knows how to supply and say which
// provider supplies each one (a rule whose provider is not connected is "not evaluated").

// FactType is the type of a fact's value.
type FactType string

// Fact types.
const (
	FactMoney    FactType = "money"
	FactDuration FactType = "duration"
	FactCount    FactType = "count"
	FactRatio    FactType = "ratio"
	FactBool     FactType = "bool"
	FactEnum     FactType = "enum"
	FactHour     FactType = "hour"
	FactSpeed    FactType = "speed"
	FactScore    FactType = "score"
	FactID       FactType = "id"
)

// FactInfo describes a fact.
type FactInfo struct {
	Type FactType
	// Input is the provider that supplies the fact.
	Input string
	// Values lists an enum's values.
	Values []string
}

// Inputs are the data providers a rule can depend on, with a plain label for the library.
var Inputs = map[string]string{ //nolint:gochecknoglobals // fixed vocabulary
	"events":               "Event data sent with each request",
	"history":              "Behaviour history kept by RiskOS (time windows and baselines)",
	"customer_profile":     "Customer and account profile (type, account age, BVN on file, KYC tier)",
	"device_binding":       "Device binding status from the mobile app",
	"device_integrity":     "Device integrity checks (rooted, jailbroken, emulator, hooking tools)",
	"session":              "Session details (open sessions, address, browser and device changes)",
	"identity":             "Identity checks (BVN and NIN live verification, liveness, NIN-phone link)",
	"telco":                "Telco signals (SIM swap, number reassignment)",
	"sanctions_list":       "Sanctions lists with fuzzy and transliteration-aware matching",
	"pep_list":             "Politically exposed persons list",
	"poi_list":             "NIBSS Person of Interest list",
	"bvn_watchlist":        "CBN BVN watchlist",
	"suspect_bvn_list":     "Temporary suspect BVN list",
	"fraud_list":           "Fraud list of reported recipients",
	"adverse_media":        "Adverse media screening",
	"fx_rates":             "Exchange rates configured by your institution",
	"ip_intelligence":      "Network address intelligence (Tor, proxy, VPN)",
	"api_gateway":          "API gateway signals (key networks, request rates, error ratios)",
	"blockchain_analytics": "Blockchain analytics wallet tags",
	"merchant_monitoring":  "Merchant refund, chargeback and terminal monitoring",
	"lending_identity":     "Loan application identity matching (photo, document, phone)",
	"staff_directory":      "Staff directory (accounts linked to employees, serviced customers)",
	"peer_groups":          "Peer-group statistics",
	"workflow":             "Alerts, cases and reports in RiskOS",
}

// Events are the event kinds a rule can be triggered by or aggregate over.
var Events = map[string]string{ //nolint:gochecknoglobals // fixed vocabulary
	"transfer_outbound":      "Outgoing transfer",
	"transfer_inbound":       "Incoming transfer",
	"transfer_international": "International transfer",
	"cash_payment":           "Cash payment",
	"cash_withdrawal":        "Cash withdrawal",
	"cash_deposit":           "Cash deposit",
	"atm_withdrawal":         "ATM withdrawal",
	"pos_purchase":           "POS purchase",
	"card_authorization":     "Card authorization",
	"crypto_purchase":        "Crypto purchase",
	"crypto_withdrawal":      "Crypto withdrawal",
	"crypto_transfer":        "Crypto transfer",
	"fiat_inflow":            "Money received for crypto",
	"funding":                "Account funding",
	"loan_application":       "Loan application",
	"loan_disbursement":      "Loan payout",
	"loan_repayment":         "Loan repayment",
	"transaction_any":        "Any money movement",
	"activity_any":           "Any account activity",
	"login":                  "Sign-in",
	"session_activity":       "Session activity",
	"auth_failure":           "Failed sign-in or verification",
	"auth_success":           "Successful verification",
	"mfa_prompt":             "Verification prompt sent",
	"beneficiary_added":      "Payee added",
	"device_bound":           "Device linked",
	"device_reregistered":    "Device re-registered",
	"sim_reregistered":       "SIM re-registered",
	"profile_change":         "Profile change",
	"account_opened":         "Account opened",
	"account_reactivated":    "Account reactivated",
	"customer_onboarded":     "Customer onboarded",
	"customer_review":        "Periodic customer review",
	"credit_requested":       "Credit requested",
	"list_updated":           "Screening list updated",
	"telco_lookup":           "Telco lookup",
	"data_export":            "Data export",
	"api_request":            "API request",
	"api_lookup":             "Account or ID lookup",
	"config_change":          "Payout or webhook settings changed",
	"role_change":            "User role changed",
	"staff_action":           "Staff action on a customer",
	"fraud_report":           "Fraud report about a recipient",
	"merchant_daily":         "Daily merchant review",
	"terminal_activity":      "Terminal activity",
	"alert_screening_due":    "Alert screening deadline reached",
	"suspicion_confirmed":    "Suspicion confirmed by an analyst",
	"report_submit":          "Report submitted",
	"daily_job":              "Daily check",
}

// Entities are what a window can be kept for ("BY").
var Entities = map[string]bool{ //nolint:gochecknoglobals // fixed vocabulary
	"account": true, "customer": true, "user": true, "device": true, "ip": true, "network": true,
	"card": true, "bvn": true, "phone": true, "address": true, "beneficiary": true, "inflow": true,
	"loan": true, "merchant": true, "api_key": true,
}

// AggFns are the aggregations, with the field type each needs ("" for none) and result type.
var AggFns = map[string]struct { //nolint:gochecknoglobals // fixed vocabulary
	Field  FactType
	Result FactType
}{
	"count":                  {"", FactCount},
	"exists":                 {"", FactBool},
	"sum":                    {FactMoney, FactMoney},
	"daily_avg":              {FactMoney, FactMoney},
	"distinct_count":         {FactID, FactCount},
	"distinct_count_once":    {FactID, FactCount},
	"max_distinct_per_value": {FactID, FactCount},
}

// Decisions a control may set (ADR-0006).
var Decisions = map[string]bool{ //nolint:gochecknoglobals // fixed vocabulary
	"decline": true, "block": true, "block_session": true, "step_up": true, "hold": true,
	"review": true, "restrict": true, "flag": true, "fee": true,
}

// MinDecisions a signal may require.
var MinDecisions = map[string]bool{"review": true, "step_up": true} //nolint:gochecknoglobals // fixed vocabulary

// Tasks a rule may create.
var Tasks = map[string]string{ //nolint:gochecknoglobals // fixed vocabulary
	"case.open":                       "Open a case",
	"close_account":                   "Close the account",
	"edd":                             "Enhanced due diligence",
	"enhanced_mfa":                    "Enhanced multi-factor authentication",
	"escalate":                        "Escalate to the compliance officer",
	"freeze_account":                  "Freeze the account",
	"fresh_auth_on_new_device":        "Fresh authentication on the new device",
	"report.CTR":                      "Currency transaction report",
	"report.FTR":                      "Foreign transaction report",
	"report.PEP_monthly":              "Monthly PEP transaction report",
	"report.STR":                      "Suspicious transaction report",
	"report.sanctions":                "Sanctions match report",
	"route_to_edd":                    "Send for enhanced checks",
	"route_via_financial_institution": "Route through a financial institution",
	"senior_approval":                 "Senior management approval",
	"source_of_funds":                 "Source of funds",
	"source_of_wealth":                "Source of wealth",
}

// Hints are responses suggested to the customer's systems.
var Hints = map[string]bool{ //nolint:gochecknoglobals // fixed vocabulary
	"alert": true, "challenge": true, "hold_and_review": true, "hold_payout": true, "pause_prompts": true,
	"rate_limit_ip": true, "reauthenticate": true, "revoke_session": true, "step_up": true,
	"temporary_lock": true, "throttle": true, "throttle_key": true,
}

// Regulators a rule can cite.
var Regulators = map[string]bool{ //nolint:gochecknoglobals // fixed vocabulary
	"CBN": true, "NFIU": true, "NCC": true, "SEC": true, "FCCPC": true, "NIBSS": true, "law": true,
}

func enum(input string, values ...string) FactInfo {
	return FactInfo{Type: FactEnum, Input: input, Values: values}
}

func fact(t FactType, input string) FactInfo { return FactInfo{Type: t, Input: input} }

// Facts are the values conditions can compare.
var Facts = map[string]FactInfo{ //nolint:gochecknoglobals // fixed vocabulary
	// The event.
	"txn.amount":                 fact(FactMoney, "events"),
	"txn.amount_usd":             fact(FactMoney, "fx_rates"),
	"txn.domestic":               fact(FactBool, "events"),
	"txn.source":                 enum("events", "card", "p2p", "bank_transfer", "cash", "wallet", "other"),
	"txn.local_hour":             fact(FactHour, "events"),
	"txn.first_to_beneficiary":   fact(FactBool, "history"),
	"txn.to_prior_sender":        fact(FactBool, "history"),
	"event.channel":              enum("events", "mobile_app", "web", "digital", "branch", "agent", "atm", "pos", "api"),
	"profile.field":              enum("events", "password", "phone", "email", "address", "name"),
	"auth.factor":                enum("events", "password", "otp", "pin", "passkey", "biometric"),
	"auth.type":                  enum("events", "api_key", "session"),
	"mfa.channel":                enum("events", "push", "otp", "sms", "email"),
	"card.result":                enum("events", "approved", "declined"),
	"crypto.external":            fact(FactBool, "events"),
	"config.field":               enum("events", "webhook_url", "payout_destination", "bank_account", "other"),
	"lookup.result":              enum("events", "found", "not_found"),
	"staff.action":               enum("events", "view_contact", "change_contact", "change_limits", "other"),
	"user.is_admin":              fact(FactBool, "events"),
	"sender.related":             fact(FactBool, "history"),
	"payer.bvn_matches_borrower": fact(FactBool, "customer_profile"),
	// Identifiers counted in aggregations.
	"account.id":        fact(FactID, "events"),
	"beneficiary.id":    fact(FactID, "events"),
	"card.id":           fact(FactID, "events"),
	"sender.id":         fact(FactID, "events"),
	"reporter.id":       fact(FactID, "events"),
	"funding_source.id": fact(FactID, "events"),
	// Customer and account profile.
	"customer.type":                            enum("customer_profile", "individual", "corporate"),
	"customer.history_age":                     fact(FactDuration, "history"),
	"account.age":                              fact(FactDuration, "customer_profile"),
	"account.has_bvn":                          fact(FactBool, "customer_profile"),
	"account.kyc_tier_norm":                    fact(FactMoney, "customer_profile"),
	"account.inactive_for":                     fact(FactDuration, "history"),
	"account.limit_raised_with_edd_and_mfa":    fact(FactBool, "customer_profile"),
	"account.linked_to_employee":               fact(FactBool, "staff_directory"),
	"account.volume_above_peer_p95":            fact(FactBool, "peer_groups"),
	"counterparty.serviced_by_linked_employee": fact(FactBool, "staff_directory"),
	// Devices and sessions.
	"device.is_new":               fact(FactBool, "history"),
	"device.bound_age":            fact(FactDuration, "device_binding"),
	"device.is_bound_device":      fact(FactBool, "device_binding"),
	"account.has_bound_device":    fact(FactBool, "device_binding"),
	"device.rooted":               fact(FactBool, "device_integrity"),
	"device.jailbroken":           fact(FactBool, "device_integrity"),
	"device.emulator":             fact(FactBool, "device_integrity"),
	"device.hooking_framework":    fact(FactBool, "device_integrity"),
	"session.open_count":          fact(FactCount, "session"),
	"session.ip_changed":          fact(FactBool, "session"),
	"session.user_agent_changed":  fact(FactBool, "session"),
	"session.device_changed":      fact(FactBool, "session"),
	"client.headless":             fact(FactBool, "session"),
	"input.keystroke_interval_ms": fact(FactCount, "session"),
	"input.timing_variance":       fact(FactRatio, "session"),
	// Beneficiaries.
	"beneficiary.is_new":        fact(FactBool, "history"),
	"beneficiary.on_fraud_list": fact(FactBool, "fraud_list"),
	"beneficiary.on_poi_list":   fact(FactBool, "poi_list"),
	// Behaviour.
	"history.amount_zscore":   fact(FactRatio, "history"),
	"history.amount_to_max":   fact(FactRatio, "history"),
	"history.hour_usual":      fact(FactBool, "history"),
	"geo.country_usual":       fact(FactBool, "history"),
	"geo.speed_from_previous": fact(FactSpeed, "history"),
	// Identity, telco and lists.
	"kyc.liveness_passed":              fact(FactBool, "identity"),
	"kyc.bvn_live_check":               enum("identity", "verified", "failed", "not_done"),
	"kyc.nin_live_check":               enum("identity", "verified", "failed", "not_done"),
	"identity.phone_nin_linked":        fact(FactBool, "identity"),
	"telco.sim_swap":                   fact(FactBool, "telco"),
	"telco.subscriber_matches_profile": fact(FactBool, "telco"),
	"screening.sanctions_score":        fact(FactScore, "sanctions_list"),
	"screening.sanctions_confirmed":    fact(FactBool, "sanctions_list"),
	"customer.is_pep":                  fact(FactBool, "pep_list"),
	"customer.has_pep_owner":           fact(FactBool, "pep_list"),
	"party.on_poi_list":                fact(FactBool, "poi_list"),
	"bvn_watchlist.category":           enum("bvn_watchlist", "0", "1", "2", "99"),
	"bvn_watchlist.active":             fact(FactBool, "bvn_watchlist"),
	"customer.bvn_on_suspect_list":     fact(FactBool, "suspect_bvn_list"),
	// Network, API and data access.
	"ip.is_tor":                      fact(FactBool, "ip_intelligence"),
	"ip.is_proxy":                    fact(FactBool, "ip_intelligence"),
	"ip.is_vpn":                      fact(FactBool, "ip_intelligence"),
	"api_key.network_known":          fact(FactBool, "api_gateway"),
	"api_key.country_known":          fact(FactBool, "api_gateway"),
	"api_key.rate_to_baseline":       fact(FactRatio, "api_gateway"),
	"api_key.error_ratio":            fact(FactRatio, "api_gateway"),
	"export.rows":                    fact(FactCount, "events"),
	"export.rows_to_user_baseline":   fact(FactRatio, "history"),
	"export.rows_to_tenant_baseline": fact(FactRatio, "history"),
	// Merchants, lending and crypto.
	"merchant.refund_rate_to_baseline":     fact(FactRatio, "merchant_monitoring"),
	"merchant.chargeback_rate_to_baseline": fact(FactRatio, "merchant_monitoring"),
	"terminal.impossible_location":         fact(FactBool, "merchant_monitoring"),
	"application.photo_reused":             fact(FactBool, "lending_identity"),
	"application.document_reused":          fact(FactBool, "lending_identity"),
	"application.phone_reused":             fact(FactBool, "lending_identity"),
	"analytics.wallet_tag":                 enum("blockchain_analytics", "sanctioned", "mixer", "darknet", "unlicensed_provider", "exchange", "unknown"),
	// Reports (REG-19).
	"report.has_predicate_offence": fact(FactBool, "workflow"),
	"report.has_indicator":         fact(FactBool, "workflow"),
	"report.category":              enum("workflow", "terrorism_financing", "kidnapping_ransom", "corruption", "fraud", "drug_trafficking", "general", "other"),
}

// paramFits reports which parameter types can stand where a fact of type t is compared.
func paramFits(t FactType) []ParamType {
	switch t {
	case FactMoney:
		return []ParamType{ParamMoney}
	case FactDuration:
		return []ParamType{ParamDuration}
	case FactCount:
		return []ParamType{ParamCount}
	case FactRatio:
		return []ParamType{ParamRatio}
	case FactHour:
		return []ParamType{ParamHour}
	case FactSpeed:
		return []ParamType{ParamSpeed}
	case FactScore:
		return []ParamType{ParamScore}
	default:
		return nil
	}
}
