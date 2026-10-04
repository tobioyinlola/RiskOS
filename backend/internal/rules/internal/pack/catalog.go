package pack

import (
	"maps"
	"slices"
)

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
	// Country packs (ADR-0007).
	"activation":         "Your institution's country activation (its licence type)",
	"wallet_profile":     "Wallet profile from your e-money system (KYC tier, balance)",
	"counterparty_tags":  "Counterparty tags and licence registers (gold buyers, crypto exchanges, licensed providers)",
	"lender_list":        "Bank of Ghana list of unlicensed digital credit providers",
	"fatf_list":          "FATF list of jurisdictions under increased monitoring",
	"agent_network":      "Agent network data (float, cash-in and cash-out by agent)",
	"incident_register":  "Incidents, breaches and vulnerabilities your teams record",
	"vendor_register":    "Vendor register (security providers, outsourcing, licences, assessments)",
	"data_governance":    "Data transfer, storage and record-keeping records (consent, safeguards, authorisations)",
	"regulator_orders":   "Orders and designations received from authorities",
	"security_programme": "Security programme records (penetration tests, assessments, audits)",
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
	// Country packs (ADR-0007).
	"forex_buy":                "Foreign currency bought",
	"forex_sell":               "Foreign currency sold",
	"forex_any":                "Any foreign currency deal",
	"cash_any":                 "Any cash deposit, withdrawal or payment",
	"wallet_cash_out":          "Wallet cash-out at an agent",
	"sim_registered":           "SIM registered",
	"sim_replaced":             "SIM replaced",
	"sim_change":               "Any SIM registration or replacement",
	"otp_used":                 "One-time password used",
	"designation_received":     "Sanctions designation received",
	"fic_order_received":       "Order received from the financial intelligence centre",
	"incident_detected":        "Incident detected",
	"vulnerability_confirmed":  "Vulnerability confirmed",
	"breach_confirmed":         "Personal-data breach confirmed",
	"outsourcing_planned":      "Outsourcing planned",
	"outsourcing_signed":       "Outsourcing agreement signed",
	"vendor_onboarded":         "Vendor onboarded",
	"vendor_review":            "Periodic vendor review",
	"remote_access_login":      "Remote access to internal networks",
	"data_transfer":            "Personal data transferred or stored",
	"record_delete_requested":  "Record deletion requested",
	"audit_log_check":          "Audit log completeness check",
	"eod_reconciliation":       "End-of-day cash reconciliation",
	"agent_daily":              "Daily agent review",
	"cdr_daily":                "Daily call-record review",
	"config_scan":              "Security configuration scan",
	"pentest_completed":        "Penetration test completed",
	"security_audit_completed": "Security audit completed",
	"audit_finding_opened":     "Audit finding opened",
	"security_review_due":      "Security programme review",
	"suspicion_recorded":       "Suspicion first recorded",
}

// Entities are what a window can be kept for ("BY").
var Entities = map[string]bool{ //nolint:gochecknoglobals // fixed vocabulary
	"account": true, "customer": true, "user": true, "device": true, "ip": true, "network": true,
	"card": true, "bvn": true, "phone": true, "address": true, "beneficiary": true, "inflow": true,
	"loan": true, "merchant": true, "api_key": true,
	// Country packs: a national ID (Ghana Card, NIDA) and a mobile-money agent.
	"national_id": true, "agent": true,
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
	// Country packs (ADR-0007).
	"report.ECTR":                  "Electronic currency transaction report",
	"report.PEP":                   "PEP transaction report",
	"report.cross_border_cash":     "Undeclared cross-border cash report",
	"report.BoG":                   "Report to the Bank of Ghana",
	"report.BNR":                   "Report to the National Bank of Rwanda",
	"report.CSA":                   "Report to the Cyber Security Authority",
	"report.DPC":                   "Breach notice to the Data Protection Commission",
	"report.NCSA":                  "Report to the National Cyber Security Authority",
	"report.RURA":                  "Report to the Rwanda Utilities Regulatory Authority",
	"notify.data_subjects":         "Tell the people affected",
	"notify.outsourcing":           "Written notice before outsourcing",
	"file.outsourcing_sla":         "File the signed service agreement",
	"biometric_verification":       "Biometric verification",
	"preserve_evidence":            "Preserve logs and evidence",
	"run.pentest":                  "Run the penetration test",
	"run.vulnerability_assessment": "Run the vulnerability assessment",
	"run.internal_audit":           "Run the internal security audit",
	"send.test_summary":            "Send the test's executive summary",
	"send.audit_report":            "Send the audit report",
	"remediate":                    "Fix the finding",
	"assess_vendor":                "Assess the vendor",
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
	// Ghana and Rwanda (ADR-0007).
	"FIC": true, "BoG": true, "CSA": true, "DPC": true, "NCA": true,
	"BNR": true, "CMA": true, "NCSA": true, "RURA": true,
}

// eventKinds lists every event kind, sorted: the values of event.kind, the event's own kind.
func eventKinds() []string { return slices.Sorted(maps.Keys(Events)) }

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
	"event.channel":              enum("events", "mobile_app", "web", "digital", "branch", "agent", "atm", "pos", "api", "ussd"),
	"profile.field":              enum("events", "password", "pin", "phone", "email", "address", "name"),
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
	// Country packs (ADR-0007). The institution and the transaction.
	"institution.licence_type": enum("activation", "bank", "sdi", "forex_bureau", "securities", "mfb", "mfi", "emi", "psp",
		"mmo", "mto", "digital_lender", "vasp", "telco", "insurer", "finance_company", "fintech", "other"),
	"txn.direction":                 enum("events", "credit", "debit"),
	"event.kind":                    {Type: FactEnum, Input: "events", Values: eventKinds()},
	"txn.from_new_sender":           fact(FactBool, "history"),
	"txn.rail":                      enum("events", "ekash", "ghipss_interop", "mobile_money", "card", "bank", "other"),
	"txn.fee":                       fact(FactMoney, "events"),
	"txn.cash_source":               enum("events", "cross_border", "domestic"),
	"txn.auth_method":               enum("events", "pin", "password", "biometric", "otp", "none"),
	"txn.shareholder_loan":          fact(FactBool, "events"),
	"txn.property_purpose":          fact(FactBool, "events"),
	"txn.amount_to_declared_income": fact(FactRatio, "customer_profile"),
	"declaration.reference_present": fact(FactBool, "events"),
	"crypto.travel_rule_complete":   fact(FactBool, "events"),
	"otp.device_mismatch":           fact(FactBool, "events"),
	"eod.cash_variance":             fact(FactMoney, "events"),
	"customer.id":                   fact(FactID, "events"),
	"agent.id":                      fact(FactID, "events"),
	// Customers, accounts and wallets.
	"account.type":                         enum("customer_profile", "mobile_money_wallet", "e_money_account", "bank_account", "other"),
	"customer.segment":                     enum("customer_profile", "individual", "sme", "corporate"),
	"customer.name_missing":                fact(FactBool, "customer_profile"),
	"customer.emoney_accounts":             fact(FactCount, "customer_profile"),
	"customer.bnr_authorisation":           fact(FactBool, "customer_profile"),
	"account.nominal_holder_without_owner": fact(FactBool, "customer_profile"),
	"account.relatives_heavy_activity":     fact(FactBool, "customer_profile"),
	"account.inflow_to_peer_p95":           fact(FactRatio, "peer_groups"),
	"beneficial_owner.identified":          fact(FactBool, "customer_profile"),
	"beneficial_owner.documents_complete":  fact(FactBool, "customer_profile"),
	"company.shell_indicators":             fact(FactCount, "customer_profile"),
	"funding.cash_share_30d":               fact(FactRatio, "customer_profile"),
	"merchant.accepts_virtual_assets":      fact(FactBool, "customer_profile"),
	"merchant.va_authorised":               fact(FactBool, "customer_profile"),
	"exception.authorised_by_bank":         fact(FactBool, "customer_profile"),
	"wallet.kyc_tier":                      enum("wallet_profile", "minimum", "medium", "enhanced"),
	"wallet.balance_after":                 fact(FactMoney, "wallet_profile"),
	// Identity and telco.
	"identity.ghana_card_verified": fact(FactBool, "identity"),
	"identity.nida_verified":       fact(FactBool, "identity"),
	"identity.sim_nida_verified":   fact(FactBool, "identity"),
	"kyc.identity_verified":        fact(FactBool, "identity"),
	"phone.foreign":                fact(FactBool, "telco"),
	"telco.sims_per_device":        fact(FactCount, "telco"),
	"telco.simbox_route_pattern":   fact(FactBool, "telco"),
	// Counterparties and lists.
	"counterparty.tag": enum("counterparty_tags", "gold_buyer", "gold_aggregator", "forex_bureau", "vasp", "p2p_crypto",
		"crypto_linked", "unlicensed_vasp", "unlicensed_lender", "real_estate", "business", "other"),
	"counterparty.business":                  fact(FactBool, "counterparty_tags"),
	"counterparty.is_vasp":                   fact(FactBool, "counterparty_tags"),
	"counterparty.vasp_licensed":             fact(FactBool, "counterparty_tags"),
	"counterparty.on_unlicensed_lender_list": fact(FactBool, "lender_list"),
	"counterparty.fatf_monitored":            fact(FactBool, "fatf_list"),
	"screening.designation_match":            fact(FactBool, "sanctions_list"),
	"order.source":                           enum("regulator_orders", "FIC", "court", "other"),
	"order.type":                             enum("regulator_orders", "suspension", "freeze", "other"),
	// Staff and agents.
	"staff.role":                     enum("staff_directory", "teller", "relationship_officer", "direct_sales_agent", "other"),
	"staff.dormant_account_debits":   fact(FactCount, "staff_directory"),
	"staff.overrides_above_peer_p95": fact(FactBool, "peer_groups"),
	"agent.float_topup_to_peer_p95":  fact(FactRatio, "agent_network"),
	"agent.cycle_to_float":           fact(FactRatio, "agent_network"),
	"agent.new_wallet_share":         fact(FactRatio, "agent_network"),
	// Incidents, vendors, sessions and data.
	"incident.kind": enum("incident_register", "security_incident", "vulnerability", "fraud", "service_interruption",
		"data_loss", "illegal_computer_use", "illegal_sim_swap"),
	"incident.severity":                     enum("incident_register", "critical", "major", "significant", "moderate", "minor"),
	"breach.personal_data":                  fact(FactBool, "incident_register"),
	"breach.unauthorised_access":            fact(FactBool, "incident_register"),
	"breach.high_risk":                      fact(FactBool, "incident_register"),
	"vendor.service":                        enum("vendor_register", "cybersecurity", "other"),
	"vendor.csa_licence_valid":              fact(FactBool, "vendor_register"),
	"vendor.due_diligence_done":             fact(FactBool, "vendor_register"),
	"vendor.assessment_age":                 fact(FactDuration, "vendor_register"),
	"session.mfa_passed":                    fact(FactBool, "session"),
	"session.compensating_control_approved": fact(FactBool, "session"),
	"data.personal":                         fact(FactBool, "data_governance"),
	"data.destination_foreign":              fact(FactBool, "data_governance"),
	"data.consent_recorded":                 fact(FactBool, "data_governance"),
	"data.safeguards_recorded":              fact(FactBool, "data_governance"),
	"data.bog_approval":                     fact(FactBool, "data_governance"),
	"data.ncsa_certificate_authorises":      fact(FactBool, "data_governance"),
	"data.ncsa_transfer_authorised":         fact(FactBool, "data_governance"),
	"data.contract_written":                 fact(FactBool, "data_governance"),
	"datastore.encrypted_at_rest":           fact(FactBool, "data_governance"),
	"link.tls_enforced":                     fact(FactBool, "data_governance"),
	"record.type":                           enum("data_governance", "audit_log", "customer_file", "transaction", "report", "other"),
	"record.age":                            fact(FactDuration, "data_governance"),
	"audit_log.coverage":                    fact(FactRatio, "data_governance"),
	"security.pentest_age":                  fact(FactDuration, "security_programme"),
	"security.vulnerability_assessment_age": fact(FactDuration, "security_programme"),
	"security.internal_audit_age":           fact(FactDuration, "security_programme"),
	"pentest.tester_certified":              fact(FactBool, "security_programme"),
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
