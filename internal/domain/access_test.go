package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseIDAcceptsStableAPIIdentifiers(t *testing.T) {
	values := []string{"abc", "case_0123", "forest_asset-row-2", "User_ABC_123"}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			id, err := ParseID("resource_id", "  "+value+"  ")
			if err != nil {
				t.Fatalf("ParseID() error = %v", err)
			}
			if id.String() != value || !id.Valid() {
				t.Fatalf("id = %q, valid=%v", id, id.Valid())
			}
		})
	}
}

func TestParseIDRejectsUnsafeIdentifiers(t *testing.T) {
	values := []string{"", "a", "has space", "slash/value", "dot.value", "unicode-灯", strings.Repeat("a", 97)}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			_, err := ParseID("resource_id", value)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("ParseID(%q) error = %v", value, err)
			}
		})
	}
}

func TestNewIDUsesPrefixAndProducesUniqueValues(t *testing.T) {
	seen := make(map[ID]bool)
	for i := 0; i < 100; i++ {
		id, err := NewID(" Case ")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(id.String(), "case_") || !id.Valid() {
			t.Fatalf("generated id = %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate generated id %s", id)
		}
		seen[id] = true
	}
}

func TestRoleValidationAndAuthorizationMatrix(t *testing.T) {
	actions := []string{"case:create", "case:view", "case:triage", "case:assign", "forest_survey:create", "forest_survey:publish", "verification:measure", "verification:close", "plan:propose", "forest_asset:complete", "plan:approve", "admin:manage"}
	wants := map[Role]map[string]bool{
		RoleResidentLiaison: {"case:create": true, "case:view": true},
		RoleOfficer:         {"case:create": true, "case:view": true, "case:triage": true, "case:assign": true, "verification:close": true, "plan:approve": true},
		RoleInspector:       {"case:create": true, "case:view": true, "forest_survey:create": true, "forest_survey:publish": true, "verification:measure": true},
		RoleOperator:        {"case:create": true, "case:view": true, "plan:propose": true, "forest_asset:complete": true},
		RoleAdmin:           {"case:create": true, "case:view": true, "case:triage": true, "case:assign": true, "forest_survey:create": true, "forest_survey:publish": true, "verification:measure": true, "verification:close": true, "plan:propose": true, "forest_asset:complete": true, "plan:approve": true, "admin:manage": true},
	}
	for role, permissions := range wants {
		t.Run(string(role), func(t *testing.T) {
			if !role.Valid() {
				t.Fatalf("role %s is invalid", role)
			}
			user := User{Role: role, Active: true}
			for _, action := range actions {
				if got, want := user.Can(action), permissions[action]; got != want {
					t.Errorf("Can(%q) = %v, want %v", action, got, want)
				}
			}
			user.Active = false
			for _, action := range actions {
				if user.Can(action) {
					t.Errorf("inactive user Can(%q) = true", action)
				}
			}
		})
	}
	if Role("root").Valid() {
		t.Fatal("unknown role is valid")
	}
}

func TestNewUserNormalizesAndCopiesPasswordHash(t *testing.T) {
	hash := []byte("password hash")
	user, err := NewUser("user_001", " Officer@Example.COM ", " Night Officer ", hash, RoleOfficer, fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	hash[0] = 'X'
	if user.Email != "officer@example.com" || user.DisplayName != "Night Officer" || string(user.PasswordHash) != "password hash" {
		t.Fatalf("user = %#v", user)
	}
	if !user.Active || user.CreatedAt.Location() != time.UTC || user.UpdatedAt.Location() != time.UTC {
		t.Fatalf("user initialization = %#v", user)
	}
}

func TestNewUserValidation(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		displayName string
		hash        []byte
		role        Role
		field       string
	}{
		{name: "invalid email", email: "not-an-email", displayName: "Officer", hash: []byte("hash"), role: RoleOfficer, field: "email"},
		{name: "short name", email: "a@example.com", displayName: "A", hash: []byte("hash"), role: RoleOfficer, field: "display_name"},
		{name: "missing hash", email: "a@example.com", displayName: "Officer", hash: nil, role: RoleOfficer, field: "password"},
		{name: "unknown role", email: "a@example.com", displayName: "Officer", hash: []byte("hash"), role: "unknown", field: "role"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewUser("user_001", test.email, test.displayName, test.hash, test.role, fixedTime())
			var fieldErr FieldError
			if !errors.As(err, &fieldErr) || fieldErr.Field != test.field {
				t.Fatalf("error = %#v, want field %s", err, test.field)
			}
		})
	}
}

func TestSessionLifecycleAndClone(t *testing.T) {
	now := fixedTime()
	revoked := now.Add(time.Hour)
	session := Session{ID: "session_001", TokenHash: []byte("secret hash"), ExpiresAt: now.Add(2 * time.Hour)}
	if !session.ActiveAt(now) || session.ActiveAt(session.ExpiresAt) {
		t.Fatalf("unexpected active state around expiry")
	}
	clone := session.Clone()
	clone.TokenHash[0] = 'X'
	if string(session.TokenHash) != "secret hash" {
		t.Fatal("clone aliases token hash")
	}
	session.RevokedAt = &revoked
	if session.ActiveAt(now) {
		t.Fatal("revoked session is active")
	}
	clone = session.Clone()
	*clone.RevokedAt = revoked.Add(time.Hour)
	if !session.RevokedAt.Equal(revoked) {
		t.Fatal("clone aliases revoked timestamp")
	}
}

func TestNewForestSiteValidationAndDefaults(t *testing.T) {
	forest_site, err := NewForestSite("forest_site_001", " Cuihu Sports Park ", "operator_001", " Luohu District ", "Asia/Shanghai", 22*60+30, fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	if forest_site.Name != "Cuihu Sports Park" || forest_site.Address != "Luohu District" || forest_site.Status != ForestSiteActive || forest_site.Version != 1 {
		t.Fatalf("forest_site = %#v", forest_site)
	}
	if forest_site.CreatedAt.Location() != time.UTC || forest_site.UpdatedAt.Location() != time.UTC {
		t.Fatalf("forest_site times are not UTC")
	}
	tests := []struct {
		name        string
		forest_site string
		address     string
		zone        string
		cutoff      int
		field       string
	}{
		{name: "short name", forest_site: "ab", address: "address", zone: "Asia/Shanghai", cutoff: 1300, field: "name"},
		{name: "missing address", forest_site: "park", address: "", zone: "Asia/Shanghai", cutoff: 1300, field: "address"},
		{name: "invalid timezone", forest_site: "park", address: "address", zone: "Mars/Base", cutoff: 1300, field: "timezone"},
		{name: "negative cutoff", forest_site: "park", address: "address", zone: "Asia/Shanghai", cutoff: -1, field: "cutoff_minute"},
		{name: "late cutoff", forest_site: "park", address: "address", zone: "Asia/Shanghai", cutoff: 1440, field: "cutoff_minute"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewForestSite("forest_site_001", test.forest_site, "operator_001", test.address, test.zone, test.cutoff, fixedTime())
			var fieldErr FieldError
			if !errors.As(err, &fieldErr) || fieldErr.Field != test.field {
				t.Fatalf("error = %#v, want field %s", err, test.field)
			}
		})
	}
}

func TestForestSiteSetCutoffUsesOptimisticVersion(t *testing.T) {
	forest_site, err := NewForestSite("forest_site_001", "Cuihu Sports Park", "operator_001", "Luohu District", "Asia/Shanghai", 1350, fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	updatedAt := fixedTime().Add(time.Hour)
	if err := forest_site.SetCutoff(1320, 1, updatedAt); err != nil {
		t.Fatal(err)
	}
	if forest_site.CutoffMinute != 1320 || forest_site.Version != 2 || !forest_site.UpdatedAt.Equal(updatedAt.UTC()) {
		t.Fatalf("forest_site = %#v", forest_site)
	}
	before := forest_site
	if err := forest_site.SetCutoff(1290, 1, updatedAt.Add(time.Hour)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale SetCutoff() error = %v", err)
	}
	if forest_site != before {
		t.Fatalf("stale update mutated forest_site: before=%#v after=%#v", before, forest_site)
	}
	if err := forest_site.SetCutoff(1440, forest_site.Version, updatedAt); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid SetCutoff() error = %v", err)
	}
}

func TestNewForestParcelValidation(t *testing.T) {
	zone, err := NewForestParcel("zone_001", " Lakeview Phase One ", " Residential address ", "liaison_001", 850, 8, fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	if zone.Name != "Lakeview Phase One" || zone.Address != "Residential address" || zone.WindowCount != 850 || zone.SensitivityLux != 8 {
		t.Fatalf("zone = %#v", zone)
	}
	tests := []struct {
		name    string
		zone    string
		address string
		windows int
		lux     float64
		field   string
	}{
		{name: "short name", zone: "x", address: "address", windows: 1, lux: 8, field: "name"},
		{name: "missing address", zone: "zone", address: "", windows: 1, lux: 8, field: "address"},
		{name: "zero windows", zone: "zone", address: "address", windows: 0, lux: 8, field: "window_count"},
		{name: "zero threshold", zone: "zone", address: "address", windows: 1, lux: 0, field: "sensitivity_lux"},
		{name: "high threshold", zone: "zone", address: "address", windows: 1, lux: 1001, field: "sensitivity_lux"},
		{name: "nan threshold", zone: "zone", address: "address", windows: 1, lux: math.NaN(), field: "sensitivity_lux"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewForestParcel("zone_001", test.zone, test.address, "liaison_001", test.windows, test.lux, fixedTime())
			var fieldErr FieldError
			if !errors.As(err, &fieldErr) || fieldErr.Field != test.field {
				t.Fatalf("error = %#v, want field %s", err, test.field)
			}
		})
	}
}

func TestNewForestAssetValidation(t *testing.T) {
	tests := []struct {
		name          string
		id            ID
		forest_siteID ID
		label         string
		row           int
		orientation   string
		angle         float64
		valid         bool
	}{
		{name: "valid", id: "forest_asset_001", forest_siteID: "forest_site_001", label: "Lamp A", row: 1, orientation: "north", angle: -30, valid: true},
		{name: "invalid id", id: "x", forest_siteID: "forest_site_001", label: "Lamp A", row: 1, orientation: "north", angle: -30},
		{name: "invalid forest_site", id: "forest_asset_001", forest_siteID: "x", label: "Lamp A", row: 1, orientation: "north", angle: -30},
		{name: "short label", id: "forest_asset_001", forest_siteID: "forest_site_001", label: "A", row: 1, orientation: "north", angle: -30},
		{name: "zero row", id: "forest_asset_001", forest_siteID: "forest_site_001", label: "Lamp A", row: 0, orientation: "north", angle: -30},
		{name: "high row", id: "forest_asset_001", forest_siteID: "forest_site_001", label: "Lamp A", row: 101, orientation: "north", angle: -30},
		{name: "short orientation", id: "forest_asset_001", forest_siteID: "forest_site_001", label: "Lamp A", row: 1, orientation: "n", angle: -30},
		{name: "low angle", id: "forest_asset_001", forest_siteID: "forest_site_001", label: "Lamp A", row: 1, orientation: "north", angle: -91},
		{name: "high angle", id: "forest_asset_001", forest_siteID: "forest_site_001", label: "Lamp A", row: 1, orientation: "north", angle: 91},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewForestAsset(test.id, test.forest_siteID, test.label, test.row, test.orientation, test.angle, fixedTime())
			if test.valid && err != nil {
				t.Fatalf("NewForestAsset() error = %v", err)
			}
			if !test.valid && !errors.Is(err, ErrValidation) {
				t.Fatalf("error = %v, want validation", err)
			}
		})
	}
}

func TestDomainErrorContracts(t *testing.T) {
	tests := []struct {
		name string
		err  error
		is   error
		text string
	}{
		{name: "field", err: FieldError{Field: "lux", Message: "must be positive"}, is: ErrValidation, text: "lux: must be positive"},
		{name: "conflict reason", err: ConflictError{Resource: "case", Reason: "already assigned"}, is: ErrConflict, text: "case: already assigned"},
		{name: "conflict default", err: ConflictError{Resource: "case"}, is: ErrConflict, text: "case conflicts with current state"},
		{name: "transition", err: TransitionError{Entity: "case", From: "open", To: "closed"}, is: ErrInvalidTransition, text: "case cannot transition from open to closed"},
		{name: "version", err: VersionConflictError{Entity: "case", Expected: 2, Actual: 3}, is: ErrConflict, text: "case version conflict: expected 2, got 3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !errors.Is(test.err, test.is) {
				t.Fatalf("errors.Is(%v, %v) = false", test.err, test.is)
			}
			if test.err.Error() != test.text {
				t.Fatalf("Error() = %q, want %q", test.err, test.text)
			}
		})
	}
}
