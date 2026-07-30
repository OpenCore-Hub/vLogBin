package domain

import (
	"errors"
	"testing"
)

func TestValidTransitions(t *testing.T) {
	valid := [][2]LifecycleState{
		{StateRegistered, StateTestActive},
		{StateTestActive, StateLiveReview},
		{StateLiveReview, StateLiveActive},
		{StateLiveReview, StateRestricted},
		{StateLiveReview, StateSuspended},
		{StateLiveActive, StateRestricted},
		{StateLiveActive, StateSuspended},
		{StateLiveActive, StateOffboarding},
		{StateRestricted, StateLiveActive},
		{StateSuspended, StateLiveActive},
	}
	for _, tr := range valid {
		if !CanTransition(tr[0], tr[1]) {
			t.Errorf("expected %s -> %s to be allowed", tr[0], tr[1])
		}
		if _, err := Transition(tr[0], tr[1]); err != nil {
			t.Errorf("Transition(%s, %s): %v", tr[0], tr[1], err)
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	invalid := [][2]LifecycleState{
		{StateRegistered, StateLiveActive},  // skip the chain
		{StateTestActive, StateLiveActive},  // must pass review first
		{StateTestActive, StateSuspended},   // not yet live
		{StateLiveActive, StateTestActive},  // no way back to test
		{StateLiveActive, StateLiveReview},  // no way back to review
		{StateOffboarding, StateLiveActive}, // terminal
		{StateTestActive, StateTestActive},  // no self-transition
		{StateLiveReview, StateRegistered},  // backwards
	}
	for _, tr := range invalid {
		if CanTransition(tr[0], tr[1]) {
			t.Errorf("expected %s -> %s to be rejected", tr[0], tr[1])
		}
		if _, err := Transition(tr[0], tr[1]); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("Transition(%s, %s) err = %v, want ErrInvalidTransition", tr[0], tr[1], err)
		}
	}
}

func TestValidScope(t *testing.T) {
	for _, s := range AllScopes {
		if !ValidScope(s) {
			t.Errorf("scope %q must be valid", s)
		}
	}
	if ValidScope("admin") {
		t.Error("unknown scope must be invalid")
	}
}
