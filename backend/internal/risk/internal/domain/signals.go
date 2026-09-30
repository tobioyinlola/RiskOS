package domain

// SignalCode identifies a built-in signal. Codes are part of the public API: never rename one.
type SignalCode = string

// Built-in signals. Default weights for the first five come from the specification (§8); the
// others are additions for account takeover and mule patterns (RISK_ENGINE.md).
const (
	SignalNewDevice              SignalCode = "new_device"
	SignalNewBeneficiary         SignalCode = "new_beneficiary"
	SignalAmountAnomaly          SignalCode = "amount_anomaly"
	SignalVelocityAnomaly        SignalCode = "velocity_anomaly"
	SignalGeoAnomaly             SignalCode = "geo_anomaly"
	SignalFailedLoginBurst       SignalCode = "failed_login_burst"
	SignalSharedDevice           SignalCode = "shared_device"
	SignalRecentCredentialChange SignalCode = "recent_credential_change" //nolint:gosec // a signal name, not a credential
	SignalBeneficiaryFanIn       SignalCode = "beneficiary_fan_in"
	SignalNewUser                SignalCode = "new_user"
)

// SignalSpec describes a built-in signal.
type SignalSpec struct {
	Code          SignalCode
	Label         string
	DefaultWeight int
	// Source is the feature source the signal depends on; if the source is unavailable the
	// signal is reported in degraded_signals instead of being silently skipped.
	Source string
	// Strengthenable signals turn a MEDIUM decision into STEP-UP: a stronger authentication step
	// directly addresses them (ARCHITECTURE.md §10).
	Strengthenable bool
}

// Feature sources.
const (
	SourceHistory  = "history"  // seen devices, beneficiaries, countries, amount baselines
	SourceVelocity = "velocity" // time-bucketed counters
	SourceNetwork  = "network"  // cross-user relationships (shared devices, fan-in)
)

// MaxSignalWeight bounds tenant weight overrides for built-in signals.
const MaxSignalWeight = 50

// BuiltinSignals returns the catalog, in a stable order.
func BuiltinSignals() []SignalSpec {
	return []SignalSpec{
		{SignalNewDevice, "New device", 20, SourceHistory, true},
		{SignalNewBeneficiary, "New beneficiary", 15, SourceHistory, false},
		{SignalAmountAnomaly, "Unusual amount", 24, SourceHistory, false},
		{SignalVelocityAnomaly, "Unusual transaction velocity", 15, SourceVelocity, false},
		{SignalGeoAnomaly, "New country for this user", 10, SourceHistory, true},
		{SignalFailedLoginBurst, "Repeated failed logins", 20, SourceVelocity, false},
		{SignalSharedDevice, "Device used by many users", 15, SourceNetwork, false},
		{SignalRecentCredentialChange, "Credentials changed recently", 20, SourceHistory, true},
		{SignalBeneficiaryFanIn, "Beneficiary receiving from many users", 15, SourceNetwork, false},
		{SignalNewUser, "New user with no history", 5, SourceHistory, false},
	}
}

// BuiltinSignal returns the spec of code.
func BuiltinSignal(code SignalCode) (SignalSpec, bool) {
	for _, s := range BuiltinSignals() {
		if s.Code == code {
			return s, true
		}
	}
	return SignalSpec{}, false
}

// Thresholds for built-in signals (engine version constants; changing one is a new engine version).
const (
	VelocityMaxPerMinute     = 3  // transactions per account per minute
	VelocityMaxPerHour       = 10 // transactions per account per hour
	FailedLoginsMax          = 5  // failed logins per user per 15 minutes
	SharedDeviceMinUsers     = 5  // distinct users on one device
	FanInMinSenders          = 10 // distinct users paying one beneficiary in 24 hours
	AmountMinHistory         = 5  // prior transactions in the currency before judging amounts
	AmountSigmas             = 3.0
	AmountMinMultiple        = 2.0
	CredentialChangeWindowHr = 24
	SeenSetCap               = 200 // max remembered devices/beneficiaries/countries per user
)
