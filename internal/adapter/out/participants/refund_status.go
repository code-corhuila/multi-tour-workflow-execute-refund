package participants

import (
	"context"

	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/application/port/out"
)

// RefundStatusUnavailable stands for the read of a refund (HU-WFL-003 G-WF3-4) until commercial-reservation serves
// `GET /internal/tenants/{tenantId}/refund-authorizations/{refundAuthorizationId}`, PROPOSED in its contract. It always
// answers an unknown failure, so an unknown outcome of step 3 ends FAILED for a person and the outflow is never reversed
// blind (AC-WF3-04, option B).
type RefundStatusUnavailable struct{}

var _ out.RefundStatusLookup = RefundStatusUnavailable{}

func (RefundStatusUnavailable) RefundStatus(context.Context, string, string) (out.RefundState, error) {
	return out.RefundState{}, unknown("STATUS_LOOKUP_UNAVAILABLE", "the refund could not be read: the route is not served yet")
}
