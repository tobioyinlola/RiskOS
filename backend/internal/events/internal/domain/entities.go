package domain

import (
	"time"

	"github.com/google/uuid"
)

// Filter narrows an event listing; empty fields match everything.
type Filter struct {
	// After returns only events newer than this event (the live monitor polls with it).
	After    *uuid.UUID
	Type     string
	Decision string
	Level    string
	User     string
	Account  string
	Device   string
}

// TxnSummary is what an analyst scans for in a transaction listing.
type TxnSummary struct {
	Amount      int64
	Currency    string
	Direction   string
	Beneficiary string
	Channel     string
}

// EntityKind is a kind of entity with a profile.
type EntityKind string

// Entity kinds.
const (
	EntityUser    EntityKind = "user"
	EntityAccount EntityKind = "account"
	EntityDevice  EntityKind = "device"
)

// ProfileWindow is how many of an entity's most recent events its profile covers.
const ProfileWindow = 5000

// Linked is another identifier seen together with an entity.
type Linked struct {
	Ref      string    `json:"ref"`
	Events   int       `json:"events"`
	LastSeen time.Time `json:"last_seen"`
}

// Profile summarizes an entity's recent activity.
type Profile struct {
	Kind                                  EntityKind
	Ref                                   string
	Events                                int
	FirstSeen, LastSeen                   time.Time
	MaxScore, MeanScore                   int
	Allowed, SteppedUp, Reviewed, Blocked int
	Users, Accounts, Devices, Countries   []Linked
	IPs                                   []Linked
}

// Point is one time bucket of analytics.
type Point struct {
	At                                     time.Time
	Events, MeanScore                      int
	Allowed, SteppedUp, Reviewed, Blocked  int
	Minimal, Low, Elevated, High, Critical int
}

// TopUser is a user ranked by their highest score in a window.
type TopUser struct {
	User                      string
	MaxScore, Events, Flagged int
	LastSeen                  time.Time
}

// Window is an analytics window with its bucket size.
type Window struct {
	Name   string
	Length time.Duration
	Bucket time.Duration
	// BucketName is "hour" or "day".
	BucketName string
}

// AnalyticsWindows are the supported analytics windows (24 or 168 hourly buckets, 30 daily).
func AnalyticsWindows() map[string]Window {
	return map[string]Window{
		"24h": {Name: "24h", Length: 24 * time.Hour, Bucket: time.Hour, BucketName: "hour"},
		"7d":  {Name: "7d", Length: 7 * 24 * time.Hour, Bucket: time.Hour, BucketName: "hour"},
		"30d": {Name: "30d", Length: 30 * 24 * time.Hour, Bucket: 24 * time.Hour, BucketName: "day"},
	}
}
