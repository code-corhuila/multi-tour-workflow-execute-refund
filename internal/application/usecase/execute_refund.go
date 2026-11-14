// Package usecase is the orchestrator of the execute-refund saga.
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/application/port/in"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/application/port/out"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/domain/saga"
)

const (
	maxDetailRunes = 500
	reversalReason = "The refund could not be completed in the reservation"
)

type ExecuteRefund struct {
	store        out.Store
	cash         out.Cash
	reservations out.Reservations
	log          *slog.Logger
	newID        func() string
	now          func() time.Time
	bound        time.Duration // longest a started saga may run, independent of the caller
}

func NewExecuteRefund(store out.Store, cash out.Cash, reservations out.Reservations,
	log *slog.Logger, newID func() string, now func() time.Time, bound time.Duration) *ExecuteRefund {
	return &ExecuteRefund{store, cash, reservations, log, newID, now, bound}
}

var _ in.ExecuteRefund = (*ExecuteRefund)(nil)

// Start runs the saga to its end. The caller leaving must not cancel a compensation half way, so the saga runs on a
// context that keeps the caller's values but not its cancellation, bounded by e.bound.
func (e *ExecuteRefund) Start(ctx context.Context, cmd in.StartCommand) (saga.Saga, bool, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.bound)
	defer cancel()
	now := e.now()
	fresh := saga.Saga{ID: e.newID(), IdempotencyKey: cmd.IdempotencyKey, Status: saga.Running, TenantID: cmd.TenantID,
		OnBehalfOf: cmd.OnBehalfOf, Refund: cmd.Refund, CreatedAt: now, UpdatedAt: now}
	s, created, err := e.store.Create(ctx, fresh)
	if err != nil {
		return saga.Saga{}, false, err
	}
	if !created {
		// A replay is the same tenant sending the same refund again. The key is unique across tenants, so a key that
		// belongs to another tenant or another refund must neither be answered nor reveal that saga.
		if s.TenantID != cmd.TenantID || s.Refund != cmd.Refund {
			return saga.Saga{}, false, saga.ErrKeyReused
		}
		return s, false, nil
	}
	return s, true, e.run(ctx, &s)
}

// logFor returns the logger of a saga: tenant and saga id on every line, never the refund data. The correlation id is
// added by the handler of the logger from the context (correlation.NewLogHandler), so the core does not import it.
func (e *ExecuteRefund) logFor(s saga.Saga) *slog.Logger {
	return e.log.With("tenantId", s.TenantID, "sagaId", s.ID)
}

func (e *ExecuteRefund) Get(ctx context.Context, id string) (saga.Saga, error) {
	return e.store.Get(ctx, id)
}

// Resume carries on the RUNNING sagas that have not advanced since before. Everything a step needs was saved with the
// saga, and every step call is idempotent by its key, so a resumed saga runs again from the first step not recorded.
//
// The stale sagas are claimed first, so only one actor takes each; a saga another actor moved meanwhile is left to
// that actor, and one that ended meanwhile is done (neither saga.ErrConflict nor saga.ErrEnded is a failure).
func (e *ExecuteRefund) Resume(ctx context.Context, before time.Time) error {
	stale, err := e.store.ClaimStale(ctx, before, e.now())
	if err != nil {
		return err
	}
	var failures []error
	for i := range stale {
		err := e.run(ctx, &stale[i])
		if errors.Is(err, saga.ErrConflict) || errors.Is(err, saga.ErrEnded) {
			e.logFor(stale[i]).InfoContext(ctx, "saga moved or ended by another actor, left to it")
			continue
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// run executes the steps not yet recorded, in order, saving the state after each one. Compensation:
//
//	START_EXECUTION      none: a failure ends FAILED, nothing was changed
//	RECORD_CASH_OUTFLOW  a definitive rejection undoes the start; an unknown outcome ends FAILED, because the money may
//	                     have moved and a person has to decide
//	COMPLETE_EXECUTION   any failure (the step is idempotent and the client already retried) reverses the outflow and
//	                     then undoes the start
//
// Crash window: if the process stops after a participant answered and before the save that follows, the step is not
// recorded; the resume repeats it with the same key, which the participant answers as a replay.
func (e *ExecuteRefund) run(ctx context.Context, s *saga.Saga) error {
	call := func(step saga.Step) out.Call { return out.Call{Key: s.StepKey(step), OnBehalfOf: s.OnBehalfOf} }

	// A saga that was compensating when the process stopped has its outflow reversed: step 3 must not run again with
	// that movement. Only a failed step 3 starts that reversal, so the compensation is taken up where it stopped.
	if s.Compensated(saga.RecordCashOutflow) {
		return e.abort(ctx, s, saga.CompleteExecution, errors.New("resumed during the compensation"), true)
	}

	if !s.Done(saga.StartExecution) {
		if err := e.reservations.StartExecution(ctx, call(saga.StartExecution), s.TenantID, s.Refund, s.OnBehalfOf); err != nil {
			s.End(saga.Failed, saga.StartExecution, sanitize(err))
			return e.finish(ctx, s)
		}
		if err := e.completed(ctx, s, saga.StartExecution); err != nil {
			return err
		}
	}
	if !s.Done(saga.RecordCashOutflow) {
		movementID, err := e.cash.RecordRefund(ctx, call(saga.RecordCashOutflow), s.TenantID, s.Refund)
		if err != nil {
			return e.abort(ctx, s, saga.RecordCashOutflow, err, definitive(err))
		}
		s.CashMovementID = movementID
		if err := e.completed(ctx, s, saga.RecordCashOutflow); err != nil {
			return err
		}
	}
	if !s.Done(saga.CompleteExecution) {
		err := e.reservations.CompleteExecution(ctx, call(saga.CompleteExecution), s.TenantID, s.Refund.RefundAuthorizationID, s.CashMovementID)
		if err != nil {
			return e.abort(ctx, s, saga.CompleteExecution, err, true)
		}
		s.Record(saga.CompleteExecution)
	}
	s.End(saga.Completed, "", "")
	return e.finish(ctx, s)
}

// abort ends the saga after a step did not succeed. When compensate is false the work may exist and a person decides.
// Otherwise the recorded steps are undone in reverse order, each save as it runs; a compensation that fails ends FAILED
// and the steps before it stay as they are.
func (e *ExecuteRefund) abort(ctx context.Context, s *saga.Saga, failed saga.Step, cause error, compensate bool) error {
	detail := sanitize(cause)
	if !compensate {
		s.End(saga.Failed, failed, detail)
		return e.finish(ctx, s)
	}
	if s.Done(saga.RecordCashOutflow) && !s.Compensated(saga.RecordCashOutflow) {
		call := out.Call{Key: s.StepKey(saga.RecordCashOutflow) + ":undo", OnBehalfOf: s.OnBehalfOf}
		if err := e.cash.ReverseRefund(ctx, call, s.TenantID, s.CashMovementID, reversalReason); err != nil {
			return e.compensationFailed(ctx, s, failed, detail, saga.RecordCashOutflow, err)
		}
		s.MarkCompensated(saga.RecordCashOutflow)
		if err := e.save(ctx, s); err != nil {
			return err
		}
	}
	call := out.Call{Key: s.StepKey(saga.StartExecution) + ":undo", OnBehalfOf: s.OnBehalfOf}
	if err := e.reservations.UndoExecution(ctx, call, s.TenantID, s.Refund.RefundAuthorizationID); err != nil {
		return e.compensationFailed(ctx, s, failed, detail, saga.StartExecution, err)
	}
	s.MarkCompensated(saga.StartExecution)
	s.End(saga.Compensated, failed, detail)
	return e.finish(ctx, s)
}

func (e *ExecuteRefund) compensationFailed(ctx context.Context, s *saga.Saga, failed saga.Step, detail string, step saga.Step, err error) error {
	code := codeOf(err)
	e.logFor(*s).WarnContext(ctx, "compensation failed", "step", step, "code", code)
	s.End(saga.Failed, failed, detail+"; compensation of "+string(step)+" failed: "+code)
	return e.finish(ctx, s)
}

func (e *ExecuteRefund) completed(ctx context.Context, s *saga.Saga, step saga.Step) error {
	s.Record(step)
	return e.save(ctx, s)
}

func (e *ExecuteRefund) save(ctx context.Context, s *saga.Saga) error {
	s.UpdatedAt = e.now()
	return e.store.Save(ctx, s)
}

func (e *ExecuteRefund) finish(ctx context.Context, s *saga.Saga) error {
	if err := e.save(ctx, s); err != nil {
		return err
	}
	e.logFor(*s).InfoContext(ctx, "saga ended", "status", s.Status, "failedStep", s.FailedStep)
	return nil
}

func definitive(err error) bool {
	var participant *saga.ParticipantError
	return errors.As(err, &participant) && participant.Definitive
}

func codeOf(err error) string {
	var participant *saga.ParticipantError
	if errors.As(err, &participant) {
		return participant.Code
	}
	return "UNKNOWN"
}

// sanitize keeps a participant's code and message and drops any other error, whose text can carry connection strings
// or tokens.
func sanitize(err error) string {
	var participant *saga.ParticipantError
	detail := "PARTICIPANT_ERROR: the participant call failed"
	if errors.As(err, &participant) {
		detail = participant.Error()
	}
	if r := []rune(detail); len(r) > maxDetailRunes {
		detail = string(r[:maxDetailRunes])
	}
	return detail
}
