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
var AllScopes = []string{ScopeRead, ScopeWrite, ScopeCredentialsManage, ScopeAuditRead}

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
