// Package saga is the execute-refund saga: its identity, status, steps and failure (Annex E).
// It imports only the standard library.
package saga

import (
	"errors"
	"time"
)

type Status string

const (
	Running     Status = "RUNNING"
	Completed   Status = "COMPLETED"
	Compensated Status = "COMPENSATED"
	Failed      Status = "FAILED"
)

type Step string

const (
	StartExecution    Step = "START_EXECUTION"
	RecordCashOutflow Step = "RECORD_CASH_OUTFLOW"
	CompleteExecution Step = "COMPLETE_EXECUTION"
)

// Medium is how the money goes out; the cash records both (cash contract, outflowMedium).
type Medium string

const (
	MediumCash     Medium = "CASH"
	MediumTransfer Medium = "TRANSFER"
)

// ErrNotFound is returned by a store when no saga has the requested id.
var ErrNotFound = errors.New("saga not found")

// ErrConflict is returned by a store when a saga is saved from a copy that is no longer the stored one: another
// actor (a resume, a live run) moved it first, and the one that lost must stop.
var ErrConflict = errors.New("saga was changed by another actor")

// ErrEnded is returned by a store when a saga that already ended is saved again: an ended saga never changes.
var ErrEnded = errors.New("saga already ended")

// ParticipantError is the only error detail a participant may hand to the saga: a code and a message
// already free of connection strings and tokens. Any other error is reduced to a generic one.
type ParticipantError struct {
	Code    string
	Message string
	// Definitive is true only for a clear rejection (a 4xx): the participant did not do the work.
	Definitive bool
}

func (e *ParticipantError) Error() string { return e.Code + ": " + e.Message }

// Refund is what the saga executes in both domains: ids, an amount and a reason, nothing else of the customer.
type Refund struct {
	RefundAuthorizationID string
	ReservationID         string
	Medium                Medium
	AmountCents           int64
	BusinessDate          string // YYYY-MM-DD, the day of the cash
	Reason                string
}

type StepState struct {
	Name        Step
	Compensated bool
}

type Saga struct {
	ID             string
	IdempotencyKey string
	Status         Status
	TenantID       string
	OnBehalfOf     string // the `sub` of the person who started it, kept so a resume can act for them
	Refund         Refund
	CashMovementID string // returned by step 2, needed by step 3 and by the reversal
	Version        int64  // counted by the store at every save and claim; a save from an older copy is refused
	Steps          []StepState
	FailedStep     Step
	FailureDetail  string // sanitized code and message only
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// StepKey is the Idempotency-Key a step sends: <saga>:<step>.
func (s Saga) StepKey(step Step) string { return s.ID + ":" + string(step) }

func (s Saga) Done(step Step) bool {
	for _, st := range s.Steps {
		if st.Name == step {
			return true
		}
	}
	return false
}

// Record marks a step as completed.
func (s *Saga) Record(step Step) { s.Steps = append(s.Steps, StepState{Name: step}) }

func (s *Saga) MarkCompensated(step Step) {
	for i := range s.Steps {
		if s.Steps[i].Name == step {
			s.Steps[i].Compensated = true
		}
	}
}

// End closes the saga. COMPENSATED and FAILED name the step that failed.
func (s *Saga) End(status Status, failed Step, detail string) {
	s.Status, s.FailedStep, s.FailureDetail = status, failed, detail
}
