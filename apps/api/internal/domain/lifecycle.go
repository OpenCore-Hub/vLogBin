// Package domain holds the Phase 0 domain model invariants: the provider
// lifecycle state machine, environment kinds, credential scopes and the
// dual commerce domains.
package domain

import (
	"errors"
	"fmt"
)

// LifecycleState is the provider lifecycle state.
type LifecycleState string

const (
	StateRegistered  LifecycleState = "REGISTERED"
	StateTestActive  LifecycleState = "TEST_ACTIVE"
	StateLiveReview  LifecycleState = "LIVE_REVIEW"
	StateLiveActive  LifecycleState = "LIVE_ACTIVE"
	StateRestricted  LifecycleState = "RESTRICTED"
	StateSuspended   LifecycleState = "SUSPENDED"
	StateOffboarding LifecycleState = "OFFBOARDING"
)

// Environment kinds. Test and Live are strictly separated.
const (
	EnvKindTest = "test"
	EnvKindLive = "live"
)

// Credential scopes.
const (
	ScopeRead              = "read"
	ScopeWrite             = "write"
	ScopeCredentialsManage = "credentials:manage"
	ScopeAuditRead         = "audit:read"
)

// AllScopes lists every scope assignable to a credential.
var AllScopes = []string{ScopeRead, ScopeWrite, ScopeCredentialsManage, ScopeAuditRead, ScopeSupportApprove, ScopeSCIMManage}

// Commerce domains. Platform commerce and provider commerce are separate
// account domains; providers never see platform-domain rows.
const (
	CommerceDomainPlatform = "platform"
	CommerceDomainProvider = "provider"
)

// ErrInvalidTransition is returned when a lifecycle transition is not
// allowed by the state machine.
var ErrInvalidTransition = errors.New("invalid lifecycle transition")

// Provider Live capabilities. Each is granted independently by the
// operator — there is no single "go live" switch (spec ID #46).
const (
	CapabilityMessaging     = "messaging"
	CapabilityDomains       = "domains"
	CapabilityPayments      = "payments"
	CapabilityThroughput    = "throughput"
	CapabilityEventDelivery = "event_delivery"
)

// AllCapabilities lists every capability an operator can grant.
var AllCapabilities = []string{
	CapabilityMessaging,
	CapabilityDomains,
	CapabilityPayments,
	CapabilityThroughput,
	CapabilityEventDelivery,
}

// ValidCapability reports whether s is a known capability.
func ValidCapability(s string) bool {
	for _, c := range AllCapabilities {
		if s == c {
			return true
		}
	}
	return false
}

// allowedTransitions encodes:
//
//	REGISTERED → TEST_ACTIVE → LIVE_REVIEW → LIVE_ACTIVE → (RESTRICTED | SUSPENDED | OFFBOARDING)
//
// with operator-controlled re-activation from RESTRICTED/SUSPENDED.
var allowedTransitions = map[LifecycleState][]LifecycleState{
	StateRegistered:  {StateTestActive},
	StateTestActive:  {StateLiveReview},
	StateLiveReview:  {StateLiveActive, StateRestricted, StateSuspended},
	StateLiveActive:  {StateRestricted, StateSuspended, StateOffboarding},
	StateRestricted:  {StateLiveActive, StateSuspended, StateOffboarding},
	StateSuspended:   {StateLiveActive, StateOffboarding},
	StateOffboarding: {},
}

// CanTransition reports whether from → to is allowed.
func CanTransition(from, to LifecycleState) bool {
	for _, s := range allowedTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Transition validates from → to and returns to, or ErrInvalidTransition.
func Transition(from, to LifecycleState) (LifecycleState, error) {
	if !CanTransition(from, to) {
		return "", fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	return to, nil
}

// ValidScope reports whether s is an assignable credential scope.
func ValidScope(s string) bool {
	for _, v := range AllScopes {
		if v == s {
			return true
		}
	}
	return false
}

// Support session access types.
const (
	SupportAccessStandard  = "standard"
	SupportAccessEmergency = "emergency"
)

// Support session statuses.
const (
	SupportStatusRequested = "requested"
	SupportStatusActive    = "active"
	SupportStatusExpired   = "expired"
	SupportStatusRevoked   = "revoked"
	SupportStatusDenied    = "denied"
)

// SupportScopeApprove is the scope required for a provider to approve
// or deny JIT support session requests on their environment.
const ScopeSupportApprove = "support:approve"

// MaxSupportSessionDuration is the maximum allowed duration for a JIT
// support session (4 hours). Emergency sessions use a tighter default.
const MaxSupportSessionDuration = 4 * 60 * 60 // seconds

// ValidSupportAccessType reports whether s is a known access type.
func ValidSupportAccessType(s string) bool {
	return s == SupportAccessStandard || s == SupportAccessEmergency
}

// Quota reservation statuses.
const (
	QuotaReserved  = "reserved"
	QuotaCommitted = "committed"
	QuotaReleased  = "released"
	QuotaExpired   = "expired"
)

// Quota period types.
const (
	QuotaPeriodDaily   = "daily"
	QuotaPeriodMonthly = "monthly"
	QuotaPeriodTotal   = "total"
)

// ValidQuotaPeriod reports whether s is a known quota period type.
func ValidQuotaPeriod(s string) bool {
	return s == QuotaPeriodDaily || s == QuotaPeriodMonthly || s == QuotaPeriodTotal
}

// Team member roles.
const (
	TeamRoleAdmin        = "admin"
	TeamRoleBillingAdmin = "billing_admin"
	TeamRoleDeveloper    = "developer"
	TeamRoleSupportAgent = "support_agent"
)

// Team member statuses.
const (
	TeamStatusActive    = "active"
	TeamStatusSuspended = "suspended"
	TeamStatusRemoved   = "removed"
)

// RoleScopes returns the scope bundle for a team role.
func RoleScopes(role string) []string {
	switch role {
	case TeamRoleAdmin:
		return []string{ScopeRead, ScopeWrite, ScopeCredentialsManage, ScopeAuditRead, ScopeSupportApprove, ScopeSCIMManage}
	case TeamRoleBillingAdmin:
		return []string{ScopeRead, ScopeWrite, ScopeAuditRead}
	case TeamRoleDeveloper:
		return []string{ScopeRead, ScopeWrite}
	case TeamRoleSupportAgent:
		return []string{ScopeRead, ScopeAuditRead, ScopeSupportApprove}
	default:
		return nil
	}
}

// ValidTeamRole reports whether s is a known team role.
func ValidTeamRole(s string) bool {
	return s == TeamRoleAdmin || s == TeamRoleBillingAdmin ||
		s == TeamRoleDeveloper || s == TeamRoleSupportAgent
}

// Migration job statuses.
const (
	MigrationStatusDraft      = "draft"
	MigrationStatusValidating = "validating"
	MigrationStatusValidated  = "validated"
	MigrationStatusImporting  = "importing"
	MigrationStatusCompleted  = "completed"
	MigrationStatusFailed     = "failed"
	MigrationStatusRolledBack = "rolled_back"
)

// Migration record statuses.
const (
	MigrationRecordPending    = "pending"
	MigrationRecordValid      = "valid"
	MigrationRecordInvalid    = "invalid"
	MigrationRecordImported   = "imported"
	MigrationRecordFailed     = "failed"
	MigrationRecordRolledBack = "rolled_back"
)

// Migration record types.
const (
	MigrationRecordCustomer     = "customer"
	MigrationRecordSubscription = "subscription"
)

// ValidMigrationRecordType reports whether s is a known record type.
func ValidMigrationRecordType(s string) bool {
	return s == MigrationRecordCustomer || s == MigrationRecordSubscription
}

// Custom domain statuses.
const (
	CustomDomainPending  = "pending"
	CustomDomainVerified = "verified"
	CustomDomainRevoked  = "revoked"
)

// DNSVerificationPrefix is the TXT record prefix for domain ownership verification.
const DNSVerificationPrefix = "_vlogbin-verify"

// Notification channels.
const (
	NotificationChannelEmail = "email"
	NotificationChannelSMS   = "sms"
)

// ValidNotificationChannel reports whether s is a known notification channel.
func ValidNotificationChannel(s string) bool {
	return s == NotificationChannelEmail || s == NotificationChannelSMS
}

// SCIM scope for managing user provisioning.
const ScopeSCIMManage = "scim:manage"

// WebhookSchemaVersion is the schema version included in webhook payloads
// and headers (spec Section 7.2: "Webhook payload 包含 schema_version").
const WebhookSchemaVersion = "1.0"

// Cell types.
const (
	CellTypeShared    = "shared"
	CellTypeDedicated = "dedicated"
)

// Cell statuses.
const (
	CellStatusActive   = "active"
	CellStatusDraining = "draining"
	CellStatusInactive = "inactive"
)

// ValidCellType reports whether s is a known cell type.
func ValidCellType(s string) bool {
	return s == CellTypeShared || s == CellTypeDedicated
}

// ValidCellStatus reports whether s is a known cell status.
func ValidCellStatus(s string) bool {
	return s == CellStatusActive || s == CellStatusDraining || s == CellStatusInactive
}

// Cell failover statuses (spec Section 14: manual failover with fencing).
const (
	FailoverStatusInitiated = "initiated"
	FailoverStatusFenced    = "fenced"
	FailoverStatusSwitched  = "switched"
	FailoverStatusReplaying = "replaying"
	FailoverStatusCompleted = "completed"
	FailoverStatusAborted   = "aborted"
)

// Cell migration statuses (spec Section 14, Phase 3: planned cell migration).
const (
	CellMigrationPlanned     = "planned"
	CellMigrationPrechecking = "prechecking"
	CellMigrationReady       = "ready"
	CellMigrationMigrating   = "migrating"
	CellMigrationCompleted   = "completed"
	CellMigrationFailed      = "failed"
	CellMigrationCancelled   = "cancelled"
)

// Metered pricing models (spec Section 18: metered billing).
const (
	PricingModelPerUnit  = "per_unit"
	PricingModelTiered   = "tiered"
	PricingModelVolume   = "volume"
	PricingModelStairStep = "stairstep"
)

// ValidPricingModel reports whether s is a known pricing model.
func ValidPricingModel(s string) bool {
	return s == PricingModelPerUnit || s == PricingModelTiered ||
		s == PricingModelVolume || s == PricingModelStairStep
}

// Budget alert statuses.
const (
	BudgetAlertOK       = "ok"
	BudgetAlertWarning  = "warning"
	BudgetAlertExceeded = "exceeded"
)
