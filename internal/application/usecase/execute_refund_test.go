package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/application/port/in"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/application/port/out"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/application/usecase"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/correlation"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/domain/saga"
)

const (
	tenant = "11111111-1111-4111-8111-111111111111"
	person = "22222222-2222-4222-8222-222222222222"
	auth   = "33333333-3333-4333-8333-333333333333"
)

var clock = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// memoryStore keeps the sagas by id and counts versions like the real store.
type memoryStore struct {
	sagas map[string]saga.Saga
	keys  map[string]string // by tenant and key, like uq_execute_refund_saga_tenant_idempotency_key
}

func newStore() *memoryStore {
	return &memoryStore{sagas: map[string]saga.Saga{}, keys: map[string]string{}}
}

func (m *memoryStore) Create(_ context.Context, s saga.Saga) (saga.Saga, bool, error) {
	key := s.TenantID + "/" + s.IdempotencyKey
	if id, ok := m.keys[key]; ok {
		return m.sagas[id], false, nil
	}
	m.keys[key], m.sagas[s.ID] = s.ID, s
	return s, true, nil
}

func (m *memoryStore) Save(_ context.Context, s *saga.Saga) error {
	stored, ok := m.sagas[s.ID]
	switch {
	case !ok:
		return saga.ErrNotFound
	case stored.Version != s.Version:
		return saga.ErrConflict
	}
	s.Version++
	m.sagas[s.ID] = *s
	return nil
}

func (m *memoryStore) Get(_ context.Context, id string) (saga.Saga, error) {
	s, ok := m.sagas[id]
	if !ok {
		return saga.Saga{}, saga.ErrNotFound
	}
	return s, nil
}

func (m *memoryStore) ClaimStale(_ context.Context, before, now time.Time) ([]saga.Saga, error) {
	var taken []saga.Saga
	for id, s := range m.sagas {
		if s.Status == saga.Running && s.UpdatedAt.Before(before) {
			s.UpdatedAt, s.Version = now, s.Version+1
			m.sagas[id] = s
			taken = append(taken, s)
		}
	}
	return taken, nil
}

// fakeCash and fakeReservations remember every call in one shared journal, so a test can check the order.
type fakeCash struct {
	journal    *[]string
	calls      []out.Call
	outflowErr error
	reverseErr error
}

func (f *fakeCash) RecordRefund(_ context.Context, call out.Call, tenantID string, r saga.Refund) (string, error) {
	*f.journal = append(*f.journal, "outflow")
	f.calls = append(f.calls, call)
	if tenantID != tenant || r.AmountCents != 80000 || r.RefundAuthorizationID != auth {
		return "", fmt.Errorf("unexpected outflow %s %d", tenantID, r.AmountCents)
	}
	return "mov-1", f.outflowErr
}

func (f *fakeCash) ReverseRefund(_ context.Context, call out.Call, _, movementID, _ string) error {
	*f.journal = append(*f.journal, "reverse:"+movementID)
	f.calls = append(f.calls, call)
	return f.reverseErr
}

type fakeReservations struct {
	journal     *[]string
	calls       []out.Call
	executedBy  string
	startErr    error
	undoErr     error
	completeErr error
}

func (f *fakeReservations) StartExecution(_ context.Context, call out.Call, _ string, _ saga.Refund, executedBy string) error {
	*f.journal = append(*f.journal, "start")
	f.calls, f.executedBy = append(f.calls, call), executedBy
	return f.startErr
}

func (f *fakeReservations) UndoExecution(_ context.Context, call out.Call, _, _ string) error {
	*f.journal = append(*f.journal, "undo")
	f.calls = append(f.calls, call)
	return f.undoErr
}

func (f *fakeReservations) CompleteExecution(_ context.Context, call out.Call, _, _, movementID string) error {
	*f.journal = append(*f.journal, "complete:"+movementID)
	f.calls = append(f.calls, call)
	return f.completeErr
}

// fakeLookup stands for the read of the refund (G-WF3-4) that follows an unknown outcome of step 3.
type fakeLookup struct {
	journal *[]string
	state   out.RefundState
	err     error
}

func (f *fakeLookup) RefundStatus(_ context.Context, tenantID, refundAuthorizationID string) (out.RefundState, error) {
	*f.journal = append(*f.journal, "lookup")
	if tenantID != tenant || refundAuthorizationID != auth {
		return out.RefundState{}, fmt.Errorf("unexpected lookup %s %s", tenantID, refundAuthorizationID)
	}
	return f.state, f.err
}

type rig struct {
	store    *memoryStore
	cash     *fakeCash
	bookings *fakeReservations
	lookup   *fakeLookup
	journal  *[]string
	log      *bytes.Buffer
	job      *usecase.ExecuteRefund
}

func newRig() *rig {
	journal := &[]string{}
	r := &rig{store: newStore(), journal: journal, log: &bytes.Buffer{}}
	r.cash, r.bookings, r.lookup = &fakeCash{journal: journal}, &fakeReservations{journal: journal}, &fakeLookup{journal: journal}
	n := 0
	r.job = usecase.NewExecuteRefund(r.store, r.cash, r.bookings, r.lookup, slog.New(correlation.NewLogHandler(slog.NewJSONHandler(r.log, nil))),
		func() string { n++; return fmt.Sprintf("saga-%d", n) }, func() time.Time { return clock }, time.Minute)
	return r
}

func command() in.StartCommand {
	return in.StartCommand{IdempotencyKey: "key-refund-0001", TenantID: tenant, OnBehalfOf: person, Refund: saga.Refund{
		RefundAuthorizationID: auth, ReservationID: "r-1", Medium: saga.MediumCash, AmountCents: 80000, BusinessDate: "2026-11-10",
		Reason: "Cancelacion"}}
}

func rejection(code string) error {
	return &saga.ParticipantError{Code: code, Message: "refused", Definitive: true}
}

func calls(r *rig) string { return strings.Join(*r.journal, ",") }

// AC-WF3-01 and AC-WF3-07: start, outflow, completion with the movement id; every call carries its key and the person.
func TestA_starts_records_the_outflow_and_completes(t *testing.T) {
	r := newRig()

	s, created, err := r.job.Start(context.Background(), command())

	if err != nil || !created || s.Status != saga.Completed {
		t.Fatalf("saga %+v created %v err %v", s, created, err)
	}
	if calls(r) != "start,outflow,complete:mov-1" {
		t.Errorf("order = %s", calls(r))
	}
	if r.bookings.calls[0].Key != "saga-1:START_EXECUTION" || r.cash.calls[0].Key != "saga-1:RECORD_CASH_OUTFLOW" ||
		r.bookings.calls[1].Key != "saga-1:COMPLETE_EXECUTION" {
		t.Errorf("keys = %v %v", r.bookings.calls, r.cash.calls)
	}
	if r.bookings.executedBy != person || r.cash.calls[0].OnBehalfOf != person {
		t.Error("the calls do not carry the person who started the saga")
	}
	if s.CashMovementID != "mov-1" || len(s.Steps) != 3 {
		t.Errorf("state = %+v", s)
	}
}

// AC-WF3-02: a refused start stops the saga before the money moves.
func TestA_a_refused_start_stops_the_saga(t *testing.T) {
	r := newRig()
	r.bookings.startErr = rejection("BUSINESS_RULE_VIOLATION")

	s, _, _ := r.job.Start(context.Background(), command())

	if s.Status != saga.Failed || s.FailedStep != saga.StartExecution || calls(r) != "start" {
		t.Errorf("saga %+v calls %s", s, calls(r))
	}
}

// AC-WF3-03: a refused outflow (the cash is not open) undoes the start.
func TestA_a_refused_outflow_undoes_the_start(t *testing.T) {
	r := newRig()
	r.cash.outflowErr = rejection("NO_OPEN_CASH")

	s, _, _ := r.job.Start(context.Background(), command())

	if s.Status != saga.Compensated || s.FailedStep != saga.RecordCashOutflow || calls(r) != "start,outflow,undo" {
		t.Errorf("saga %+v calls %s", s, calls(r))
	}
	if last := r.bookings.calls[len(r.bookings.calls)-1]; last.Key != "saga-1:START_EXECUTION:undo" {
		t.Errorf("undo key = %q", last.Key)
	}
}

// An unknown outcome of the outflow may have moved the money: nothing is undone and a person decides.
func TestA_an_unknown_outflow_outcome_is_left_failed(t *testing.T) {
	r := newRig()
	r.cash.outflowErr = errors.New("timeout")

	s, _, _ := r.job.Start(context.Background(), command())

	if s.Status != saga.Failed || strings.Contains(calls(r), "undo") || strings.Contains(calls(r), "reverse") {
		t.Errorf("saga %+v calls %s", s, calls(r))
	}
}

// AC-WF3-04 (option B): a completion refused for good (4xx) reverses the outflow and undoes the start, without reading
// the refund.
func TestA_a_refused_completion_is_compensated_without_reading_the_refund(t *testing.T) {
	r := newRig()
	r.bookings.completeErr = rejection("VALIDATION_ERROR")

	s, _, _ := r.job.Start(context.Background(), command())

	if s.Status != saga.Compensated || s.FailedStep != saga.CompleteExecution || calls(r) != "start,outflow,complete:mov-1,reverse:mov-1,undo" {
		t.Errorf("saga %+v calls %s", s, calls(r))
	}
	if !s.Steps[0].Compensated || !s.Steps[1].Compensated {
		t.Errorf("steps = %+v", s.Steps)
	}
}

// AC-WF3-04 (option B): an unknown completion that the reservation shows EXECUTED with the movement of step 2 completes
// the saga; nothing is reversed.
func TestA_an_unknown_completion_found_executed_completes(t *testing.T) {
	r := newRig()
	r.bookings.completeErr = errors.New("timeout")
	r.lookup.state = out.RefundState{ExecutionState: out.RefundExecuted, CashMovementID: "mov-1"}

	s, _, err := r.job.Start(context.Background(), command())

	if err != nil || s.Status != saga.Completed || s.FailedStep != "" || calls(r) != "start,outflow,complete:mov-1,lookup" {
		t.Errorf("saga %+v err %v calls %s", s, err, calls(r))
	}
	if !s.Done(saga.CompleteExecution) || s.Compensated(saga.RecordCashOutflow) {
		t.Errorf("steps = %+v", s.Steps)
	}
}

// AC-WF3-04 (option B): an unknown completion that the reservation shows not executed (EXECUTING after step 1, or
// AUTHORIZED) reverses the outflow and undoes the start.
func TestA_an_unknown_completion_found_not_executed_is_compensated(t *testing.T) {
	for _, state := range []string{out.RefundExecuting, out.RefundAuthorized} {
		r := newRig()
		r.bookings.completeErr = &saga.ParticipantError{Code: "PARTICIPANT_UNAVAILABLE", Message: "the participant answered 503"}
		r.lookup.state = out.RefundState{ExecutionState: state}

		s, _, _ := r.job.Start(context.Background(), command())

		if s.Status != saga.Compensated || s.FailedStep != saga.CompleteExecution ||
			calls(r) != "start,outflow,complete:mov-1,lookup,reverse:mov-1,undo" {
			t.Errorf("%s: saga %+v calls %s", state, s, calls(r))
		}
	}
}

// AC-WF3-04 (option B), norm section 5.8: when the read fails too, nothing is reversed nor undone and a person decides.
func TestA_an_unknown_completion_whose_read_fails_is_left_failed(t *testing.T) {
	r := newRig()
	r.bookings.completeErr = errors.New("timeout")
	r.lookup.err = &saga.ParticipantError{Code: "STATUS_LOOKUP_UNAVAILABLE", Message: "the refund could not be read"}

	s, _, err := r.job.Start(context.Background(), command())

	if err != nil || s.Status != saga.Failed || s.FailedStep != saga.CompleteExecution || calls(r) != "start,outflow,complete:mov-1,lookup" {
		t.Errorf("saga %+v err %v calls %s", s, err, calls(r))
	}
	if s.Compensated(saga.RecordCashOutflow) || s.Compensated(saga.StartExecution) || s.FailureDetail == "" {
		t.Errorf("saga %+v", s)
	}
}

// AC-WF3-04 (option B): EXECUTED with a movement other than the one of step 2 is not the outcome of this saga; a person
// decides.
func TestA_an_unknown_completion_executed_with_another_movement_is_left_failed(t *testing.T) {
	r := newRig()
	r.bookings.completeErr = errors.New("timeout")
	r.lookup.state = out.RefundState{ExecutionState: out.RefundExecuted, CashMovementID: "mov-other"}

	s, _, _ := r.job.Start(context.Background(), command())

	if s.Status != saga.Failed || s.FailedStep != saga.CompleteExecution || calls(r) != "start,outflow,complete:mov-1,lookup" {
		t.Errorf("saga %+v calls %s", s, calls(r))
	}
}

// AC-WF3-05: a compensation that fails leaves the saga FAILED, with the code and no internal detail.
func TestA_a_failed_compensation_leaves_the_saga_failed(t *testing.T) {
	r := newRig()
	r.bookings.completeErr = rejection("INVALID_STATUS_TRANSITION")
	r.cash.reverseErr = rejection("ALREADY_CLOSED")

	s, _, _ := r.job.Start(context.Background(), command())

	if s.Status != saga.Failed || s.FailedStep != saga.CompleteExecution || !strings.Contains(s.FailureDetail, "ALREADY_CLOSED") {
		t.Errorf("saga %+v", s)
	}
	if strings.Contains(calls(r), "undo") {
		t.Errorf("the start was undone while the outflow is still recorded: %s", calls(r))
	}
}

// AC-WF3-06: the same key starts nothing twice.
func TestA_a_second_start_is_a_replay(t *testing.T) {
	r := newRig()
	r.job.Start(context.Background(), command())
	before := len(*r.journal)

	s, created, err := r.job.Start(context.Background(), command())

	if err != nil || created || s.ID != "saga-1" || len(*r.journal) != before {
		t.Errorf("created %v err %v calls %v", created, err, *r.journal)
	}
}

// AC-WF3-08: a stale saga is claimed and finished from its saved state, without calling the earlier steps again.
func TestA_a_stale_saga_is_resumed_once(t *testing.T) {
	r := newRig()
	old := clock.Add(-10 * time.Minute)
	r.store.sagas["s-9"] = saga.Saga{ID: "s-9", Status: saga.Running, TenantID: tenant, OnBehalfOf: person, UpdatedAt: old,
		Refund: command().Refund, CashMovementID: "mov-9",
		Steps: []saga.StepState{{Name: saga.StartExecution}, {Name: saga.RecordCashOutflow}}}

	if err := r.job.Resume(context.Background(), clock.Add(-2*time.Minute)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := r.job.Resume(context.Background(), clock.Add(-2*time.Minute)); err != nil {
		t.Fatalf("second resume: %v", err)
	}

	got := r.store.sagas["s-9"]
	if got.Status != saga.Completed || calls(r) != "complete:mov-9" {
		t.Errorf("saga %+v calls %s", got, calls(r))
	}
}

// AC-WF3-09: an error that is not a participant's never reaches the state or the log.
func TestA_logs_and_state_carry_no_secret(t *testing.T) {
	r := newRig()
	r.bookings.startErr = errors.New("dial postgres://user:secret-password@db:5432: refused")

	s, _, _ := r.job.Start(context.Background(), command())

	if strings.Contains(s.FailureDetail, "secret-password") || strings.Contains(r.log.String(), "secret-password") {
		t.Errorf("a secret leaked: %q %s", s.FailureDetail, r.log.String())
	}
}

func TestA_get_reads_the_saga_of_the_store(t *testing.T) {
	r := newRig()
	started, _, _ := r.job.Start(context.Background(), command())

	got, err := r.job.Get(context.Background(), started.ID)

	if err != nil || got.ID != started.ID {
		t.Errorf("got %+v err %v", got, err)
	}
}

// D5: a resume after a crash in the middle of a compensation finishes the compensation; it does not run step 3 again.
func TestA_a_resume_in_the_middle_of_a_compensation_does_not_complete_the_refund(t *testing.T) {
	r := newRig()
	old := clock.Add(-10 * time.Minute)
	r.store.sagas["s-9"] = saga.Saga{ID: "s-9", Status: saga.Running, TenantID: tenant, OnBehalfOf: person, UpdatedAt: old,
		Refund: command().Refund, CashMovementID: "mov-9",
		Steps: []saga.StepState{{Name: saga.StartExecution}, {Name: saga.RecordCashOutflow, Compensated: true}}}

	if err := r.job.Resume(context.Background(), clock.Add(-2*time.Minute)); err != nil {
		t.Fatalf("resume: %v", err)
	}

	got := r.store.sagas["s-9"]
	if got.Status != saga.Compensated || calls(r) != "undo" {
		t.Errorf("saga %+v calls %s", got, calls(r))
	}
}

// AC-WF3-06 amended (DD-23): the same tenant with the same key and another refund is refused and shows nothing.
func TestA_the_same_key_with_another_body_is_refused(t *testing.T) {
	changed := command()
	changed.Refund.AmountCents = 1
	r := newRig()
	r.job.Start(context.Background(), command())
	before := len(*r.journal)

	s, created, err := r.job.Start(context.Background(), changed)

	if !errors.Is(err, saga.ErrKeyReused) || created || s.ID != "" || len(*r.journal) != before {
		t.Errorf("saga %+v created %v err %v", s, created, err)
	}
}

// AC-WF3-06 amended (DD-23): the key is per tenant, so another tenant with the same key starts its own saga; it is never
// refused, which would tell it that another tenant used the key, and it never sees the first saga.
func TestA_the_key_of_another_tenant_starts_its_own_saga(t *testing.T) {
	other := command()
	other.TenantID = "99999999-9999-4999-8999-999999999999"
	r := newRig()
	first, _, _ := r.job.Start(context.Background(), command())

	s, created, err := r.job.Start(context.Background(), other)

	if err != nil || !created || s.ID == first.ID || s.TenantID != other.TenantID || s.Status != saga.Completed {
		t.Errorf("saga %+v created %v err %v", s, created, err)
	}
}

// D3 and AC-WF3-09: the log of the saga shows the tenant, the saga id and the correlation id, and no secret.
func TestA_the_log_carries_tenant_saga_and_correlation_id(t *testing.T) {
	r := newRig()
	ctx := correlation.With(context.Background(), "corr-0001")

	r.job.Start(ctx, command())

	logged := r.log.String()
	for _, want := range []string{tenant, "saga-1", "corr-0001"} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log lacks %q: %s", want, logged)
		}
	}
	if strings.Contains(logged, "Cancelacion") {
		t.Errorf("the free-text reason reached the log: %s", logged)
	}
}
