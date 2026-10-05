package rules

// FieldInfo describes a fact for people who build rules in the console without writing code:
// what it is called, where it belongs, how its values read and which events carry it.
type FieldInfo struct {
	Label string
	Group string
	// Unit is how a number reads: "money" (minor units of the event's currency), "count" or
	// "ratio"; empty for strings and booleans.
	Unit string
	// Kind says how a free-text string is picked: "country" or "currency" (ISO codes).
	Kind string
	// Values are the only values a string field takes, with their labels.
	Values []Choice
	// Events are the event types that carry the fact; empty means every event.
	Events []string
}

// Choice is one value of a field, with its label.
type Choice struct{ Value, Label string }

// Field groups, in the order the console lists them.
const (
	GroupEvent       = "Event"
	GroupTransaction = "Transaction"
	GroupLogin       = "Login"
	GroupHistory     = "Customer history"
	GroupActivity    = "Recent activity"
)

// Info describes every fact of Fields.
func Info() map[string]FieldInfo {
	onTransaction, onLogin := []string{"transaction"}, []string{"login"}
	return map[string]FieldInfo{
		"event.type": {Label: "Event type", Group: GroupEvent, Values: []Choice{
			{"transaction", "Transaction"}, {"login", "Login"}, {"beneficiary_added", "Beneficiary added"},
			{"device_change", "Device change"}, {"session_change", "Session change"},
			{"profile_change", "Profile change"}, {"activity", "Other activity"},
		}},
		"context.country":      {Label: "Country the event came from", Group: GroupEvent, Kind: "country"},
		"transaction.amount":   {Label: "Amount", Group: GroupTransaction, Unit: "money", Events: onTransaction},
		"transaction.currency": {Label: "Currency", Group: GroupTransaction, Kind: "currency", Events: onTransaction},
		"transaction.direction": {Label: "Direction", Group: GroupTransaction, Events: onTransaction, Values: []Choice{
			{"debit", "Money out"}, {"credit", "Money in"},
		}},
		"transaction.channel": {Label: "Channel", Group: GroupTransaction, Events: onTransaction, Values: []Choice{
			{"card", "Card"}, {"bank_transfer", "Bank transfer"}, {"instant_payment", "Instant payment"},
			{"wallet", "Wallet"}, {"other", "Other"},
		}},
		"beneficiary.country": {Label: "Beneficiary's country", Group: GroupTransaction, Kind: "country",
			Events: []string{"transaction", "beneficiary_added"}},
		"login.outcome": {Label: "Login result", Group: GroupLogin, Events: onLogin, Values: []Choice{
			{"success", "Succeeded"}, {"failure", "Failed"},
		}},
		"login.method": {Label: "Login method", Group: GroupLogin, Events: onLogin, Values: []Choice{
			{"password", "Password"}, {"passkey", "Passkey"}, {"otp", "One-time code"},
			{"sso", "Single sign-on"}, {"biometric", "Biometric"},
		}},
		"profile.credential_change": {Label: "Email, phone or password changed", Group: GroupEvent,
			Events: []string{"profile_change"}},
		"feature.new_user":                 {Label: "First time this customer is seen", Group: GroupHistory},
		"feature.new_device":               {Label: "New device for this customer", Group: GroupHistory},
		"feature.new_beneficiary":          {Label: "New beneficiary for this customer", Group: GroupHistory},
		"feature.new_country":              {Label: "New country for this customer", Group: GroupHistory},
		"feature.recent_credential_change": {Label: "Email, phone or password changed in the last 24 hours", Group: GroupHistory},
		"feature.amount_to_mean_ratio": {Label: "Amount compared with this customer's usual amount", Group: GroupHistory,
			Unit: "ratio", Events: onTransaction},
		"feature.account_txn_1m": {Label: "Transactions by this account in the last minute", Group: GroupActivity,
			Unit: "count", Events: onTransaction},
		"feature.account_txn_1h": {Label: "Transactions by this account in the last hour", Group: GroupActivity,
			Unit: "count", Events: onTransaction},
		"feature.account_txn_24h": {Label: "Transactions by this account in the last 24 hours", Group: GroupActivity,
			Unit: "count", Events: onTransaction},
		"feature.failed_logins_15m": {Label: "Failed logins in the last 15 minutes", Group: GroupActivity,
			Unit: "count", Events: onLogin},
		"feature.device_users": {Label: "Customers seen on this device", Group: GroupActivity, Unit: "count"},
		"feature.beneficiary_senders_24h": {Label: "Customers who paid this beneficiary in the last 24 hours",
			Group: GroupActivity, Unit: "count", Events: onTransaction},
	}
}
