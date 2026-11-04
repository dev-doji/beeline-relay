package payment

import "testing"

var allStates = []State{
	StateAwaitingApproval, StateAwaitingAuthorization, StateSubmitted, StateProcessing,
	StateRecoveryRequired, StateCompleted, StateFailed, StateRefunded, StateExpired,
}

// The full transition matrix. Every (from, to) pair must have an explicit expectation, so
// adding a state without deciding its transitions fails this test.
var matrix = map[State]map[State]Decision{
	StateAwaitingApproval: {
		StateAwaitingApproval: Duplicate, StateAwaitingAuthorization: Apply, StateSubmitted: Invalid,
		StateProcessing: Invalid, StateRecoveryRequired: Invalid, StateCompleted: Invalid,
		StateFailed: Apply, StateRefunded: Invalid, StateExpired: Apply,
	},
	StateAwaitingAuthorization: {
		StateAwaitingApproval: Stale, StateAwaitingAuthorization: Duplicate, StateSubmitted: Apply,
		StateProcessing: Apply, StateRecoveryRequired: Apply, StateCompleted: Apply,
		StateFailed: Apply, StateRefunded: Apply, StateExpired: Apply,
	},
	StateSubmitted: {
		StateAwaitingApproval: Stale, StateAwaitingAuthorization: Stale, StateSubmitted: Duplicate,
		StateProcessing: Apply, StateRecoveryRequired: Apply, StateCompleted: Apply,
		StateFailed: Apply, StateRefunded: Apply, StateExpired: Invalid,
	},
	StateProcessing: {
		StateAwaitingApproval: Stale, StateAwaitingAuthorization: Stale, StateSubmitted: Stale,
		StateProcessing: Duplicate, StateRecoveryRequired: Apply, StateCompleted: Apply,
		StateFailed: Apply, StateRefunded: Apply, StateExpired: Invalid,
	},
	StateRecoveryRequired: {
		StateAwaitingApproval: Stale, StateAwaitingAuthorization: Stale, StateSubmitted: Stale,
		StateProcessing: Apply, StateRecoveryRequired: Duplicate, StateCompleted: Apply,
		StateFailed: Apply, StateRefunded: Apply, StateExpired: Invalid,
	},
}

func init() {
	for _, terminal := range []State{StateCompleted, StateFailed, StateRefunded, StateExpired} {
		row := map[State]Decision{}
		for _, to := range allStates {
			switch {
			case to == terminal:
				row[to] = Duplicate
			case to.Terminal():
				row[to] = Conflict
			default:
				row[to] = Stale
			}
		}
		matrix[terminal] = row
	}
}

func TestTransitionMatrix(t *testing.T) {
	for _, from := range allStates {
		for _, to := range allStates {
			want, ok := matrix[from][to]
			if !ok {
				t.Fatalf("matrix missing expectation for %s -> %s", from, to)
			}
			if got := Decide(from, to); got != want {
				t.Errorf("Decide(%s, %s) = %s, want %s", from, to, got, want)
			}
		}
	}
}

func TestCompletedNeverRegresses(t *testing.T) {
	for _, to := range allStates {
		if to != StateCompleted && Decide(StateCompleted, to) == Apply {
			t.Errorf("completed must not move to %s", to)
		}
	}
}

func TestSubmissionAloneNeverCompletes(t *testing.T) {
	// Completion must come from an observation, never be implied by submission: the only
	// state "submitted" leads to automatically is itself.
	if Decide(StateAwaitingAuthorization, StateSubmitted) != Apply {
		t.Fatal("submission should be recordable")
	}
	if StateSubmitted.Terminal() {
		t.Fatal("submitted must not be terminal")
	}
}
