package payment

// State is the normalized payment lifecycle state. The provider-native status is kept
// separately on the payment and in event metadata.
type State string

const (
	StateAwaitingApproval      State = "awaiting_approval"
	StateAwaitingAuthorization State = "awaiting_authorization"
	StateSubmitted             State = "submitted"
	StateProcessing            State = "processing"
	StateRecoveryRequired      State = "recovery_required"
	StateCompleted             State = "completed"
	StateFailed                State = "failed"
	StateRefunded              State = "refunded"
	StateExpired               State = "expired"
)

func (s State) Terminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateRefunded, StateExpired:
		return true
	}
	return false
}

// allowed lists every legal transition. Anything not listed is rejected.
var allowed = map[State][]State{
	StateAwaitingApproval:      {StateAwaitingAuthorization, StateFailed, StateExpired},
	StateAwaitingAuthorization: {StateSubmitted, StateProcessing, StateCompleted, StateFailed, StateRefunded, StateExpired, StateRecoveryRequired},
	StateSubmitted:             {StateProcessing, StateCompleted, StateFailed, StateRefunded, StateRecoveryRequired},
	StateProcessing:            {StateCompleted, StateFailed, StateRefunded, StateRecoveryRequired},
	StateRecoveryRequired:      {StateProcessing, StateCompleted, StateFailed, StateRefunded},
}

// progress orders the non-terminal states so an older observation can be recognised.
var progress = map[State]int{
	StateAwaitingApproval:      0,
	StateAwaitingAuthorization: 1,
	StateSubmitted:             2,
	StateProcessing:            3,
	StateRecoveryRequired:      3,
}

// Decision is what the state machine does with an observed target state.
type Decision string

const (
	// Apply: move to the target state.
	Apply Decision = "apply"
	// Duplicate: already in the target state; record nothing new.
	Duplicate Decision = "duplicate"
	// Stale: an older observation arrived late; keep the current state.
	Stale Decision = "stale"
	// Conflict: a different terminal outcome than the one already recorded. The current
	// state is kept and the contradiction is recorded for an operator.
	Conflict Decision = "conflict"
	// Invalid: a transition that is never legal (a bug or a bad request).
	Invalid Decision = "invalid"
)

// Decide is the single place that decides lifecycle transitions.
func Decide(from, to State) Decision {
	if from == to {
		return Duplicate
	}
	if from.Terminal() {
		if to.Terminal() {
			return Conflict
		}
		return Stale // a terminal state never regresses
	}
	for _, s := range allowed[from] {
		if s == to {
			return Apply
		}
	}
	if pf, ok := progress[from]; ok {
		if pt, ok := progress[to]; ok && pt < pf {
			return Stale
		}
	}
	return Invalid
}
