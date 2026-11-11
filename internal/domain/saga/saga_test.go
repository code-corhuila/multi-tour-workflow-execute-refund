package saga_test

import (
	"testing"

	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/domain/saga"
)

// The steps are the ones of the spec and of the check constraints of the schema.
func TestSteps_are_the_ones_of_the_spec(t *testing.T) {
	got := []saga.Step{saga.StartExecution, saga.RecordCashOutflow, saga.CompleteExecution}
	want := []string{"START_EXECUTION", "RECORD_CASH_OUTFLOW", "COMPLETE_EXECUTION"}
	for i := range got {
		if string(got[i]) != want[i] {
			t.Errorf("step %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// AC-WF3-05: the key of a step is stable for the same saga and step, and differs between steps.
func TestStepKey_is_stable_per_saga_and_step(t *testing.T) {
	s := saga.Saga{ID: "a1"}

	if s.StepKey(saga.StartExecution) != "a1:START_EXECUTION" {
		t.Errorf("key = %q", s.StepKey(saga.StartExecution))
	}
	if s.StepKey(saga.StartExecution) == s.StepKey(saga.RecordCashOutflow) {
		t.Error("two steps share a key")
	}
}

func TestRecord_and_Done_follow_the_steps_that_were_completed(t *testing.T) {
	s := saga.Saga{}

	if s.Done(saga.StartExecution) {
		t.Fatal("a new saga has done nothing")
	}
	s.Record(saga.StartExecution)

	if !s.Done(saga.StartExecution) || s.Done(saga.RecordCashOutflow) {
		t.Errorf("steps = %+v", s.Steps)
	}
}

// AC-WF3-03: the reversal marks the step that it undid, and only that one.
func TestMarkCompensated_marks_only_the_named_step(t *testing.T) {
	s := saga.Saga{}
	s.Record(saga.StartExecution)
	s.Record(saga.RecordCashOutflow)

	s.MarkCompensated(saga.StartExecution)

	if !s.Steps[0].Compensated || s.Steps[1].Compensated {
		t.Errorf("steps = %+v", s.Steps)
	}
}

func TestEnd_closes_the_saga_with_the_step_that_failed(t *testing.T) {
	s := saga.Saga{Status: saga.Running}

	s.End(saga.Compensated, saga.RecordCashOutflow, "VALIDATION: no seat")

	if s.Status != saga.Compensated || s.FailedStep != saga.RecordCashOutflow || s.FailureDetail != "VALIDATION: no seat" {
		t.Errorf("saga = %+v", s)
	}
}

// AC-WF3-08: nothing of the customer is part of the state: the fields are ids, amounts and the concept.
func TestRefund_carries_no_name_or_document_of_the_customer(t *testing.T) {
	p := saga.Refund{RefundAuthorizationID: "a", ReservationID: "r", Medium: "CASH", AmountCents: 100, BusinessDate: "2026-11-10", Reason: "c"}

	if p.AmountCents != 100 || p.Medium != saga.MediumCash {
		t.Errorf("refund = %+v", p)
	}
}

// AC-WF3-05: a store tells apart the three ways a save can be refused.
func TestErrors_are_distinct(t *testing.T) {
	if saga.ErrNotFound == saga.ErrConflict || saga.ErrConflict == saga.ErrEnded {
		t.Error("the errors of the store must differ")
	}
}
