package participants

import (
	"context"
	"net/http"
	"net/url"

	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/application/port/out"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/domain/saga"
)

const reservationsPrefix = "/api/v1/commercial-reservation/internal/tenants/"

// Reservations implements out.Reservations over the internal routes of commercial-reservation (HU-COM-016 plan, PROPOSED).
// Every step is idempotent by state: repeating it with the same effect answers 200.
type Reservations struct{ c *client }

var _ out.Reservations = (*Reservations)(nil)

func NewReservations(baseURL string, cfg Config) *Reservations {
	return &Reservations{newClient(baseURL, cfg)}
}

func route(tenantID, authorizationID, step string) string {
	return reservationsPrefix + url.PathEscape(tenantID) + "/refund-authorizations/" + url.PathEscape(authorizationID) + "/" + step
}

// StartExecution is step 1: the service refuses a refund with no prior authorization.
func (r *Reservations) StartExecution(ctx context.Context, call out.Call, tenantID string, f saga.Refund, executedBy string) error {
	_, err := r.c.post(ctx, request{
		path: route(tenantID, f.RefundAuthorizationID, "execution-start"),
		key:  call.Key,
		body: map[string]string{"executedBy": executedBy, "outflowMethod": string(f.Medium)},
		ok:   []int{http.StatusOK},
	})
	return err
}

// UndoExecution is the compensation of step 1; the service reads nothing from the body.
func (r *Reservations) UndoExecution(ctx context.Context, call out.Call, tenantID, refundAuthorizationID string) error {
	_, err := r.c.post(ctx, request{
		path: route(tenantID, refundAuthorizationID, "execution-undo"),
		key:  call.Key,
		body: struct{}{},
		ok:   []int{http.StatusOK},
	})
	return err
}

// CompleteExecution is step 3: the refund becomes EXECUTED with the movement of the cash.
func (r *Reservations) CompleteExecution(ctx context.Context, call out.Call, tenantID, refundAuthorizationID, movementID string) error {
	_, err := r.c.post(ctx, request{
		path: route(tenantID, refundAuthorizationID, "execution-completion"),
		key:  call.Key,
		body: map[string]string{"cashMovementId": movementID},
		ok:   []int{http.StatusOK},
	})
	return err
}
