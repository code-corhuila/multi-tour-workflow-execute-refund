package participants

import (
	"context"
	"net/http"
	"net/url"

	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/application/port/out"
	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/domain/saga"
)

const cashPrefix = "/api/v1/cash-reporting/internal/tenants/"

// Cash implements out.Cash over the internal routes of cash-reporting (contract of docs #840 and #848). The tenant is the
// one of the path; onBehalfOf travels in the body, where the contract accepts it from a service token only (ADR-004 D5).
type Cash struct{ c *client }

var _ out.Cash = (*Cash)(nil)

func NewCash(baseURL string, cfg Config) *Cash { return &Cash{newClient(baseURL, cfg)} }

// RecordRefund returns the id of the movement that step 3 and the compensation will name.
func (c *Cash) RecordRefund(ctx context.Context, call out.Call, tenantID string, r saga.Refund) (string, error) {
	body, err := c.c.post(ctx, request{
		path: cashPrefix + url.PathEscape(tenantID) + "/cash-refunds",
		key:  call.Key,
		body: map[string]any{"businessDate": r.BusinessDate, "outflowMedium": string(r.Medium), "amountCents": r.AmountCents,
			"reservationId": r.ReservationID, "refundAuthorizationId": r.RefundAuthorizationID, "reason": r.Reason,
			"onBehalfOf": call.OnBehalfOf},
		ok: []int{http.StatusCreated, http.StatusOK},
	})
	if err != nil {
		return "", err
	}
	return decode(body, "cashMovementId")
}

// ReverseRefund records the compensating movement; the outflow is never updated or deleted.
func (c *Cash) ReverseRefund(ctx context.Context, call out.Call, tenantID, movementID, reason string) error {
	_, err := c.c.post(ctx, request{
		path: cashPrefix + url.PathEscape(tenantID) + "/cash-movements/" + url.PathEscape(movementID) + "/reversals",
		key:  call.Key,
		body: map[string]string{"reason": reason, "onBehalfOf": call.OnBehalfOf},
		ok:   []int{http.StatusCreated, http.StatusOK},
	})
	return err
}
