// Package out holds the ports the orchestrator needs: one per participant, and the saga store.
package out

import (
	"context"
	"time"

	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/domain/saga"
)

// Call is what every participant call carries besides its data.
type Call struct {
	Key        string // Idempotency-Key: <saga>:<step>
	OnBehalfOf string // the `sub` of the person who started the saga (ADR-004 D5)
}

// Cash is the participant of step 2 and of its compensation (cash-reporting, internal routes). An error that is not a
// *saga.ParticipantError is treated as an unknown failure.
type Cash interface {
	RecordRefund(ctx context.Context, call Call, tenantID string, r saga.Refund) (movementID string, err error)
	// ReverseRefund records a compensating movement; the outflow itself is never updated or deleted.
	ReverseRefund(ctx context.Context, call Call, tenantID, movementID, reason string) error
}

// Reservations is the participant of steps 1 and 3 and of the compensation of step 1 (commercial-reservation, internal routes).
type Reservations interface {
	StartExecution(ctx context.Context, call Call, tenantID string, r saga.Refund, executedBy string) error
	UndoExecution(ctx context.Context, call Call, tenantID, refundAuthorizationID string) error
	CompleteExecution(ctx context.Context, call Call, tenantID, refundAuthorizationID, movementID string) error
}

// Execution states of a refund in commercial-reservation (RefundExecutionState of its contract).
const (
	RefundAuthorized = "AUTHORIZED"
	RefundExecuting  = "EXECUTING"
	RefundExecuted   = "EXECUTED"
)

// RefundState is what the saga needs to know of a refund after an unknown outcome of step 3.
type RefundState struct {
	ExecutionState string
	CashMovementID string // set when EXECUTED
}

// RefundStatusLookup reads a refund without changing it (HU-WFL-003 G-WF3-4, route PROPOSED in the commercial contract).
// An error that is not a *saga.ParticipantError is treated as an unknown failure.
type RefundStatusLookup interface {
	RefundStatus(ctx context.Context, tenantID, refundAuthorizationID string) (RefundState, error)
}

// Store persists the saga. Create inserts a RUNNING saga or, when the idempotency key exists, returns that one
// with created=false.
type Store interface {
	Create(ctx context.Context, s saga.Saga) (stored saga.Saga, created bool, err error)
	// Save writes the saga if it is still the stored copy (same Version) and raises s.Version; otherwise it answers
	// saga.ErrConflict (moved by another actor), saga.ErrEnded or saga.ErrNotFound.
	Save(ctx context.Context, s *saga.Saga) error
	Get(ctx context.Context, id string) (saga.Saga, error)
	// ClaimStale takes the RUNNING sagas not updated since before: it stamps them with now and raises their Version in
	// one step, so no other caller takes the same saga, and returns them as taken.
	ClaimStale(ctx context.Context, before, now time.Time) ([]saga.Saga, error)
}
