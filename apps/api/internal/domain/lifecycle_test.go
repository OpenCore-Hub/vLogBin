package domain

import (
	"errors"
	"testing"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from LifecycleState
		to   LifecycleState
		want bool
	}{
		{"registered to test rejected", StateRegistered, StateTestActive, false},
		{"test to review", StateTestActive, StateLiveReview, true},
		{"review to active", StateLiveReview, StateLiveActive, true},
		{"active to restricted", StateLiveActive, StateRestricted, true},
		{"active to suspended", StateLiveActive, StateSuspended, true},
		{"active to offboarding", StateLiveActive, StateOffboarding, true},
		{"restricted to active", StateRestricted, StateLiveActive, true},
		{"suspended to active", StateSuspended, StateLiveActive, true},
		{"suspended to offboarding", StateSuspended, StateOffboarding, true},
		// Invalid transitions
		{"registered to live", StateRegistered, StateLiveActive, false},
		{"registered to review", StateRegistered, StateLiveReview, false},
		{"offboarding to active", StateOffboarding, StateLiveActive, false},
		{"test to suspended", StateTestActive, StateSuspended, false},
		{"unknown state", LifecycleState("UNKNOWN"), StateRegistered, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestTransition(t *testing.T) {
	t.Run("valid transition", func(t *testing.T) {
		got, err := Transition(StateTestActive, StateLiveReview)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != StateLiveReview {
			t.Fatalf("got %v, want %v", got, StateLiveReview)
		}
	})

	t.Run("invalid transition", func(t *testing.T) {
		_, err := Transition(StateRegistered, StateLiveActive)
		if !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("expected ErrInvalidTransition, got %v", err)
		}
	})

	// REGISTERED → TEST_ACTIVE is exclusively the activation flow's job
	// (assigns region + cell, provisions the test environment); the generic
	// transition must reject it.
	t.Run("registered to test rejected", func(t *testing.T) {
		_, err := Transition(StateRegistered, StateTestActive)
		if !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("expected ErrInvalidTransition, got %v", err)
		}
	})
}

func TestCanWrite(t *testing.T) {
	tests := []struct {
		name  string
		state LifecycleState
		want  bool
	}{
		{"registered", StateRegistered, false},
		{"test active", StateTestActive, true},
		{"live review", StateLiveReview, true},
		{"live active", StateLiveActive, true},
		{"restricted", StateRestricted, true},
		{"suspended", StateSuspended, false},
		{"offboarding", StateOffboarding, false},
		{"unknown", LifecycleState("UNKNOWN"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanWrite(tt.state); got != tt.want {
				t.Errorf("CanWrite(%s) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestCanPublishCatalog(t *testing.T) {
	tests := []struct {
		name  string
		state LifecycleState
		want  bool
	}{
		{"registered", StateRegistered, false},
		{"test active", StateTestActive, true},
		{"live review", StateLiveReview, true},
		{"live active", StateLiveActive, true},
		{"restricted", StateRestricted, true},
		{"suspended", StateSuspended, false},
		{"offboarding", StateOffboarding, false},
		{"unknown", LifecycleState("UNKNOWN"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanPublishCatalog(tt.state); got != tt.want {
				t.Errorf("CanPublishCatalog(%s) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestAllowedTransitions(t *testing.T) {
	// Verify AllowedTransitions returns a copy (modifying it must not
	// affect the internal state machine).
	tests := []struct {
		from     LifecycleState
		expected []LifecycleState
	}{
		{StateRegistered, []LifecycleState{}},
		{StateTestActive, []LifecycleState{StateLiveReview}},
		{StateLiveReview, []LifecycleState{StateLiveActive, StateRestricted, StateSuspended}},
		{StateLiveActive, []LifecycleState{StateRestricted, StateSuspended, StateOffboarding}},
		{StateRestricted, []LifecycleState{StateLiveActive, StateSuspended, StateOffboarding}},
		{StateSuspended, []LifecycleState{StateLiveActive, StateOffboarding}},
		{StateOffboarding, []LifecycleState{}},
	}

	for _, tt := range tests {
		t.Run(string(tt.from), func(t *testing.T) {
			got := AllowedTransitions(tt.from)
			if len(got) != len(tt.expected) {
				t.Fatalf("AllowedTransitions(%s) = %v, want %v", tt.from, got, tt.expected)
			}
			for i, want := range tt.expected {
				if got[i] != want {
					t.Errorf("AllowedTransitions(%s)[%d] = %v, want %v", tt.from, i, got[i], want)
				}
			}
			// Mutate the returned slice and verify the internal map
			// is unaffected (proves it's a copy).
			if len(got) > 0 {
				got[0] = LifecycleState("MUTATED")
				fresh := AllowedTransitions(tt.from)
				if fresh[0] == "MUTATED" {
					t.Fatal("AllowedTransitions did not return a copy; internal state machine was mutated")
				}
			}
		})
	}

	// Unknown state returns nil (empty).
	if got := AllowedTransitions(LifecycleState("UNKNOWN")); len(got) != 0 {
		t.Fatalf("AllowedTransitions(UNKNOWN) = %v, want nil/empty", got)
	}
}

func TestValidScope(t *testing.T) {
	valid := []string{ScopeRead, ScopeWrite, ScopeCredentialsManage, ScopeAuditRead, ScopeSupportApprove, ScopeSCIMManage}
	for _, s := range valid {
		if !ValidScope(s) {
			t.Errorf("ValidScope(%q) = false, want true", s)
		}
	}

	invalid := []string{"", "admin", "superuser", "read-write", "scopes"}
	for _, s := range invalid {
		if ValidScope(s) {
			t.Errorf("ValidScope(%q) = true, want false", s)
		}
	}
}

func TestValidCapability(t *testing.T) {
	for _, c := range AllCapabilities {
		if !ValidCapability(c) {
			t.Errorf("ValidCapability(%q) = false, want true", c)
		}
	}

	if ValidCapability("unknown") {
		t.Error("ValidCapability(unknown) should be false")
	}
	if ValidCapability("") {
		t.Error("ValidCapability(empty) should be false")
	}
}

func TestRoleScopes(t *testing.T) {
	tests := []struct {
		role     string
		minCount int
		hasRead  bool
	}{
		{TeamRoleAdmin, 6, true},
		{TeamRoleBillingAdmin, 3, true},
		{TeamRoleDeveloper, 2, true},
		{TeamRoleSupportAgent, 3, true},
		{"unknown", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			scopes := RoleScopes(tt.role)
			if len(scopes) < tt.minCount {
				t.Errorf("RoleScopes(%q) returned %d scopes, want >= %d", tt.role, len(scopes), tt.minCount)
			}
			foundRead := false
			for _, s := range scopes {
				if s == ScopeRead {
					foundRead = true
				}
			}
			if foundRead != tt.hasRead {
				t.Errorf("RoleScopes(%q) has read=%v, want %v", tt.role, foundRead, tt.hasRead)
			}
		})
	}
}

func TestValidTeamRole(t *testing.T) {
	valid := []string{TeamRoleAdmin, TeamRoleBillingAdmin, TeamRoleDeveloper, TeamRoleSupportAgent}
	for _, r := range valid {
		if !ValidTeamRole(r) {
			t.Errorf("ValidTeamRole(%q) = false, want true", r)
		}
	}

	if ValidTeamRole("superadmin") {
		t.Error("ValidTeamRole(superadmin) should be false")
	}
	if ValidTeamRole("") {
		t.Error("ValidTeamRole(empty) should be false")
	}
}

func TestValidSupportAccessType(t *testing.T) {
	if !ValidSupportAccessType(SupportAccessStandard) {
		t.Error("standard should be valid")
	}
	if !ValidSupportAccessType(SupportAccessEmergency) {
		t.Error("emergency should be valid")
	}
	if ValidSupportAccessType("unknown") {
		t.Error("unknown should be invalid")
	}
}

func TestValidQuotaPeriod(t *testing.T) {
	valid := []string{QuotaPeriodDaily, QuotaPeriodMonthly, QuotaPeriodTotal}
	for _, p := range valid {
		if !ValidQuotaPeriod(p) {
			t.Errorf("ValidQuotaPeriod(%q) = false", p)
		}
	}
	if ValidQuotaPeriod("hourly") {
		t.Error("hourly should be invalid")
	}
}

func TestValidMigrationRecordType(t *testing.T) {
	if !ValidMigrationRecordType(MigrationRecordCustomer) {
		t.Error("customer should be valid")
	}
	if !ValidMigrationRecordType(MigrationRecordSubscription) {
		t.Error("subscription should be valid")
	}
	if ValidMigrationRecordType("invoice") {
		t.Error("invoice should be invalid")
	}
}

func TestValidNotificationChannel(t *testing.T) {
	if !ValidNotificationChannel(NotificationChannelEmail) {
		t.Error("email should be valid")
	}
	if !ValidNotificationChannel(NotificationChannelSMS) {
		t.Error("sms should be valid")
	}
	if ValidNotificationChannel("push") {
		t.Error("push should be invalid")
	}
}

func TestValidCellType(t *testing.T) {
	if !ValidCellType(CellTypeShared) {
		t.Error("shared should be valid")
	}
	if !ValidCellType(CellTypeDedicated) {
		t.Error("dedicated should be valid")
	}
	if ValidCellType("hybrid") {
		t.Error("hybrid should be invalid")
	}
}

func TestValidCellStatus(t *testing.T) {
	valid := []string{CellStatusActive, CellStatusDraining, CellStatusInactive}
	for _, s := range valid {
		if !ValidCellStatus(s) {
			t.Errorf("ValidCellStatus(%q) = false", s)
		}
	}
	if ValidCellStatus("unknown") {
		t.Error("unknown should be invalid")
	}
}

func TestValidPricingModel(t *testing.T) {
	valid := []string{PricingModelPerUnit, PricingModelTiered, PricingModelVolume, PricingModelStairStep}
	for _, m := range valid {
		if !ValidPricingModel(m) {
			t.Errorf("ValidPricingModel(%q) = false", m)
		}
	}
	if ValidPricingModel("fixed") {
		t.Error("fixed should be invalid")
	}
}

func TestAllScopesComplete(t *testing.T) {
	// Ensure AllScopes contains all scope constants.
	expected := map[string]bool{
		ScopeRead:              false,
		ScopeWrite:             false,
		ScopeCredentialsManage: false,
		ScopeAuditRead:         false,
		ScopeSupportApprove:    false,
		ScopeSCIMManage:        false,
	}
	for _, s := range AllScopes {
		expected[s] = true
	}
	for scope, found := range expected {
		if !found {
			t.Errorf("AllScopes missing %q", scope)
		}
	}
}

func TestAllCapabilitiesComplete(t *testing.T) {
	expected := map[string]bool{
		CapabilityMessaging:     false,
		CapabilityDomains:       false,
		CapabilityPayments:      false,
		CapabilityThroughput:    false,
		CapabilityEventDelivery: false,
	}
	for _, c := range AllCapabilities {
		expected[c] = true
	}
	for cap, found := range expected {
		if !found {
			t.Errorf("AllCapabilities missing %q", cap)
		}
	}
}

func TestMaxSupportSessionDuration(t *testing.T) {
	if MaxSupportSessionDuration != 4*60*60 {
		t.Fatalf("MaxSupportSessionDuration = %d, want %d", MaxSupportSessionDuration, 4*60*60)
	}
}

func TestWebhookSchemaVersion(t *testing.T) {
	if WebhookSchemaVersion != "1.0" {
		t.Fatalf("WebhookSchemaVersion = %q, want 1.0", WebhookSchemaVersion)
	}
}

func TestDNSVerificationPrefix(t *testing.T) {
	if DNSVerificationPrefix != "_vlogbin-verify" {
		t.Fatalf("DNSVerificationPrefix = %q, want _vlogbin-verify", DNSVerificationPrefix)
	}
}
