package participants_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/adapter/out/participants"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/application/port/out"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/correlation"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/domain/saga"
)

const (
	token  = "s3cret-service-token"
	tenant = "11111111-1111-4111-8111-111111111111"
	person = "22222222-2222-4222-8222-222222222222"
	auth   = "33333333-3333-4333-8333-333333333333"
)

var cfg = participants.Config{Token: token, Timeout: time.Second, Attempts: 3}

// stub stands for a participant: the routes of the cash api (docs #840 and #848) and of the commercial api (HU-COM-016 plan)
// may not be on their develop yet. It answers the scripted statuses in order and remembers the last call.
type stub struct {
	statuses []int
	body     string
	calls    atomic.Int32
	method   string
	path     string
	raw      string
	header   http.Header
	sent     map[string]any
}

func newStub(t *testing.T, body string, statuses ...int) (*httptest.Server, *stub) {
	t.Helper()
	f := &stub{statuses: statuses, body: body}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(f.calls.Add(1))
		raw, _ := io.ReadAll(r.Body)
		f.sent = map[string]any{}
		_ = json.Unmarshal(raw, &f.sent)
		f.method, f.path, f.raw, f.header = r.Method, r.URL.Path, r.URL.EscapedPath(), r.Header.Clone()
		status := http.StatusOK
		if n <= len(f.statuses) {
			status = f.statuses[n-1]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status < 300 {
			_, _ = w.Write([]byte(f.body))
			return
		}
		_, _ = w.Write([]byte(`{"error":"NO_OPEN_CASH","message":"The cash is not open","traceId":"t"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, f
}

func ctx() context.Context { return correlation.With(context.Background(), "run-1234") }

func refund() saga.Refund {
	return saga.Refund{RefundAuthorizationID: auth, ReservationID: "r-1", Medium: saga.MediumTransfer, AmountCents: 80000,
		BusinessDate: "2026-11-10", Reason: "Cancelacion"}
}

var call = out.Call{Key: "saga-1:RECORD_CASH_OUTFLOW", OnBehalfOf: person}

// AC-WF3-01, -06, -07: the outflow goes to the internal route of the tenant, with the key, the credential, the correlation id
// and onBehalfOf in the body; the id of the movement comes back.
func TestI_the_outflow_is_posted_to_the_internal_route(t *testing.T) {
	srv, stub := newStub(t, `{"cashMovementId":"mov-1"}`, http.StatusCreated)

	id, err := participants.NewCash(srv.URL, cfg).RecordRefund(ctx(), call, tenant, refund())

	if err != nil || id != "mov-1" {
		t.Fatalf("id %q err %v", id, err)
	}
	if stub.method != http.MethodPost || stub.path != "/api/v1/cash-reporting/internal/tenants/"+tenant+"/cash-refunds" {
		t.Errorf("asked %s %s", stub.method, stub.path)
	}
	if stub.header.Get("Authorization") != "Bearer "+token || stub.header.Get("Idempotency-Key") != call.Key ||
		stub.header.Get("X-Correlation-Id") != "run-1234" {
		t.Errorf("headers = %v", stub.header)
	}
	want := map[string]any{"businessDate": "2026-11-10", "outflowMedium": "TRANSFER", "amountCents": float64(80000),
		"reservationId": "r-1", "refundAuthorizationId": auth, "reason": "Cancelacion", "onBehalfOf": person}
	for k, v := range want {
		if stub.sent[k] != v {
			t.Errorf("body %s = %v, want %v", k, stub.sent[k], v)
		}
	}
	if len(stub.sent) != len(want) {
		t.Errorf("body = %v", stub.sent)
	}
}

// AC-WF3-03: a 4xx is a definitive rejection with the code of the cash.
func TestI_a_4xx_is_a_definitive_rejection(t *testing.T) {
	srv, stub := newStub(t, `{}`, http.StatusUnprocessableEntity)

	_, err := participants.NewCash(srv.URL, cfg).RecordRefund(ctx(), call, tenant, refund())

	var p *saga.ParticipantError
	if !errors.As(err, &p) || !p.Definitive || p.Code != "NO_OPEN_CASH" || stub.calls.Load() != 1 {
		t.Errorf("err %v calls %d", err, stub.calls.Load())
	}
}

// AC-WF3-05: network errors, 429 and 5xx are retried a bounded number of times and then are an unknown outcome.
func TestI_5xx_is_retried_and_then_unknown(t *testing.T) {
	srv, stub := newStub(t, `{"cashMovementId":"mov-1"}`, 503, 429, 201)
	if id, err := participants.NewCash(srv.URL, cfg).RecordRefund(ctx(), call, tenant, refund()); err != nil || id != "mov-1" || stub.calls.Load() != 3 {
		t.Errorf("id %q err %v calls %d", id, err, stub.calls.Load())
	}

	srv, stub = newStub(t, `{}`, 503, 503, 503)
	_, err := participants.NewCash(srv.URL, cfg).RecordRefund(ctx(), call, tenant, refund())
	var p *saga.ParticipantError
	if !errors.As(err, &p) || p.Definitive || stub.calls.Load() != 3 || strings.Contains(err.Error(), token) {
		t.Errorf("err %v calls %d", err, stub.calls.Load())
	}
}

// A 2xx without the id of the movement is an unknown outcome: step 3 could not name the movement.
func TestI_an_answer_without_the_movement_id_is_unknown(t *testing.T) {
	srv, _ := newStub(t, `{}`, http.StatusCreated)

	_, err := participants.NewCash(srv.URL, cfg).RecordRefund(ctx(), call, tenant, refund())

	var p *saga.ParticipantError
	if !errors.As(err, &p) || p.Definitive {
		t.Errorf("err %v", err)
	}
}

// AC-WF3-04: the reversal of the outflow names the movement in the path (route of docs #848).
func TestI_the_reversal_is_posted_to_the_movement(t *testing.T) {
	srv, stub := newStub(t, `{"cashMovementId":"mov-2"}`, http.StatusCreated)

	err := participants.NewCash(srv.URL, cfg).ReverseRefund(ctx(), out.Call{Key: "saga-1:RECORD_CASH_OUTFLOW:undo", OnBehalfOf: person}, tenant, "mov-1", "why")

	if err != nil || stub.path != "/api/v1/cash-reporting/internal/tenants/"+tenant+"/cash-movements/mov-1/reversals" {
		t.Fatalf("err %v path %s", err, stub.path)
	}
	if stub.sent["reason"] != "why" || stub.sent["onBehalfOf"] != person || stub.header.Get("Idempotency-Key") != "saga-1:RECORD_CASH_OUTFLOW:undo" {
		t.Errorf("sent %v headers %v", stub.sent, stub.header)
	}
}

// AC-WF3-01 and -07: step 1 posts the person and the method to the authorization of the tenant.
func TestI_the_start_is_posted_with_the_person_and_the_method(t *testing.T) {
	srv, stub := newStub(t, `{}`, http.StatusOK)

	err := participants.NewReservations(srv.URL, cfg).StartExecution(ctx(), out.Call{Key: "saga-1:START_EXECUTION", OnBehalfOf: person}, tenant, refund(), person)

	if err != nil || stub.path != "/api/v1/commercial-reservation/internal/tenants/"+tenant+"/refund-authorizations/"+auth+"/execution-start" {
		t.Fatalf("err %v path %s", err, stub.path)
	}
	if stub.sent["executedBy"] != person || stub.sent["outflowMethod"] != "TRANSFER" || len(stub.sent) != 2 {
		t.Errorf("sent %v", stub.sent)
	}
}

// AC-WF3-03: the undo has no body field; the amount and the person are never read from it.
func TestI_the_undo_is_posted_without_fields(t *testing.T) {
	srv, stub := newStub(t, `{}`, http.StatusOK)

	err := participants.NewReservations(srv.URL, cfg).UndoExecution(ctx(), call, tenant, auth)

	if err != nil || stub.path != "/api/v1/commercial-reservation/internal/tenants/"+tenant+"/refund-authorizations/"+auth+"/execution-undo" || len(stub.sent) != 0 {
		t.Errorf("err %v path %s sent %v", err, stub.path, stub.sent)
	}
}

// AC-WF3-01: step 3 posts the movement of the cash.
func TestI_the_completion_is_posted_with_the_movement(t *testing.T) {
	srv, stub := newStub(t, `{}`, http.StatusOK)

	err := participants.NewReservations(srv.URL, cfg).CompleteExecution(ctx(), call, tenant, auth, "mov-1")

	if err != nil || stub.path != "/api/v1/commercial-reservation/internal/tenants/"+tenant+"/refund-authorizations/"+auth+"/execution-completion" {
		t.Fatalf("err %v path %s", err, stub.path)
	}
	if stub.sent["cashMovementId"] != "mov-1" || len(stub.sent) != 1 {
		t.Errorf("sent %v", stub.sent)
	}
}

// An id of the path that is not a plain id cannot change the route.
func TestI_the_ids_of_the_path_are_escaped(t *testing.T) {
	srv, stub := newStub(t, `{}`, http.StatusOK)

	_ = participants.NewReservations(srv.URL, cfg).UndoExecution(ctx(), call, tenant, "../x")

	if strings.Contains(stub.raw, "/../") || !strings.Contains(stub.raw, "%2F") {
		t.Errorf("path = %s", stub.raw)
	}
}

// A redirect is an unknown outcome and is never followed: it would replay the body and the credential elsewhere.
func TestI_a_redirect_is_not_followed(t *testing.T) {
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)

	err := participants.NewReservations(srv.URL, cfg).CompleteExecution(ctx(), call, tenant, auth, "mov-1")

	var p *saga.ParticipantError
	if !errors.As(err, &p) || p.Definitive || hits.Load() != 0 {
		t.Errorf("err %v hits %d", err, hits.Load())
	}
}
