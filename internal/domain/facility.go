package domain

import (
	"math"
	"strings"
	"time"
)

type ForestSiteStatus string

const (
	ForestSiteActive     ForestSiteStatus = "active"
	ForestSiteRestricted ForestSiteStatus = "restricted"
	ForestSiteSuspended  ForestSiteStatus = "suspended"
)

type ForestSite struct {
	ID           ID
	Name         string
	OperatorID   ID
	Address      string
	Timezone     string
	Status       ForestSiteStatus
	Latitude     float64
	Longitude    float64
	CutoffMinute int
	Version      int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewForestSite(id ID, name string, operatorID ID, address, timezone string, cutoffMinute int, now time.Time) (ForestSite, error) {
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	if len(name) < 3 || len(name) > 120 {
		return ForestSite{}, FieldError{Field: "name", Message: "must contain 3 to 120 characters"}
	}
	if address == "" {
		return ForestSite{}, FieldError{Field: "address", Message: "is required"}
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return ForestSite{}, FieldError{Field: "timezone", Message: "is invalid"}
	}
	if cutoffMinute < 0 || cutoffMinute >= 24*60 {
		return ForestSite{}, FieldError{Field: "cutoff_minute", Message: "must be within a day"}
	}
	now = now.UTC()
	return ForestSite{ID: id, Name: name, OperatorID: operatorID, Address: address, Timezone: timezone, Status: ForestSiteActive, CutoffMinute: cutoffMinute, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (f *ForestSite) SetCutoff(minute int, expectedVersion int64, now time.Time) error {
	if minute < 0 || minute >= 24*60 {
		return FieldError{Field: "cutoff_minute", Message: "must be within a day"}
	}
	if f.Version != expectedVersion {
		return VersionConflictError{Entity: "forest_site", Expected: expectedVersion, Actual: f.Version}
	}
	f.CutoffMinute = minute
	f.Version++
	f.UpdatedAt = now.UTC()
	return nil
}

type ForestParcel struct {
	ID             ID
	Name           string
	Address        string
	ContactUserID  ID
	WindowCount    int
	SensitivityLux float64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewForestParcel(id ID, name, address string, contact ID, windowCount int, sensitivityLux float64, now time.Time) (ForestParcel, error) {
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	if len(name) < 2 || len(name) > 120 {
		return ForestParcel{}, FieldError{Field: "name", Message: "must contain 2 to 120 characters"}
	}
	if address == "" {
		return ForestParcel{}, FieldError{Field: "address", Message: "is required"}
	}
	if windowCount <= 0 {
		return ForestParcel{}, FieldError{Field: "window_count", Message: "must be positive"}
	}
	if math.IsNaN(sensitivityLux) || math.IsInf(sensitivityLux, 0) || sensitivityLux <= 0 || sensitivityLux > 1000 {
		return ForestParcel{}, FieldError{Field: "sensitivity_lux", Message: "must be between 0 and 1000"}
	}
	now = now.UTC()
	return ForestParcel{ID: id, Name: name, Address: address, ContactUserID: contact, WindowCount: windowCount, SensitivityLux: sensitivityLux, CreatedAt: now, UpdatedAt: now}, nil
}

type ForestAsset struct {
	ID           ID
	ForestSiteID ID
	Label        string
	RowNumber    int
	Orientation  string
	Enabled      bool
	Shielded     bool
	AngleDegrees float64
	Version      int64
	UpdatedAt    time.Time
}

func NewForestAsset(id, forest_siteID ID, label string, rowNumber int, orientation string, angleDegrees float64, now time.Time) (ForestAsset, error) {
	label = strings.TrimSpace(label)
	orientation = strings.TrimSpace(orientation)
	if !id.Valid() || !forest_siteID.Valid() {
		return ForestAsset{}, FieldError{Field: "id", Message: "forest_asset and forest_site identifiers are required"}
	}
	if len(label) < 2 || len(label) > 80 {
		return ForestAsset{}, FieldError{Field: "label", Message: "must contain 2 to 80 characters"}
	}
	if rowNumber < 1 || rowNumber > 100 {
		return ForestAsset{}, FieldError{Field: "row_number", Message: "must be between 1 and 100"}
	}
	if len(orientation) < 2 || len(orientation) > 80 {
		return ForestAsset{}, FieldError{Field: "orientation", Message: "must contain 2 to 80 characters"}
	}
	if math.IsNaN(angleDegrees) || math.IsInf(angleDegrees, 0) || angleDegrees < -90 || angleDegrees > 90 {
		return ForestAsset{}, FieldError{Field: "angle_degrees", Message: "must be between -90 and 90"}
	}
	now = now.UTC()
	return ForestAsset{ID: id, ForestSiteID: forest_siteID, Label: label, RowNumber: rowNumber, Orientation: orientation, Enabled: true, AngleDegrees: angleDegrees, Version: 1, UpdatedAt: now}, nil
}

func (f ForestAsset) Clone() ForestAsset { return f }
