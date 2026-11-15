package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/code-corhuila/multi-tour-workflow-execute-refund/internal/domain/saga"
)

const maxBodyBytes = 16 << 10

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// startRequest has no field for the tenant or the person who executes: both come from the token (ADR-003, ADR-004 D5).
// amountCents is a number kept raw, so a decimal or a string is refused instead of rounded.
type startRequest struct {
	RefundAuthorizationID string          `json:"refundAuthorizationId"`
	ReservationID         string          `json:"reservationId"`
	OutflowMedium         string          `json:"outflowMedium"`
	AmountCents           json.RawMessage `json:"amountCents"`
	BusinessDate          string          `json:"businessDate"`
	Reason                string          `json:"reason"`
}

type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// decodeStart reads the body strictly: one JSON object, no field the contract does not declare. The messages name the
// field and the rule, never the value.
func decodeStart(r *http.Request, w http.ResponseWriter) (saga.Refund, []fieldError) {
	var req startRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return saga.Refund{}, []fieldError{{"body", "must be one JSON object with only the declared fields"}}
	}
	var problems []fieldError
	check := func(ok bool, field, message string) {
		if !ok {
			problems = append(problems, fieldError{field, message})
		}
	}
	var amount int64
	amountErr := json.Unmarshal(req.AmountCents, &amount)
	_, dateErr := time.Parse(time.DateOnly, req.BusinessDate)
	check(uuidPattern.MatchString(req.RefundAuthorizationID), "refundAuthorizationId", "must be a lower case UUID")
	check(uuidPattern.MatchString(req.ReservationID), "reservationId", "must be a lower case UUID")
	check(req.OutflowMedium == string(saga.MediumCash) || req.OutflowMedium == string(saga.MediumTransfer), "outflowMedium", "must be CASH or TRANSFER")
	check(amountErr == nil && amount >= 1, "amountCents", "must be a whole number of cents, at least 1")
	check(dateErr == nil, "businessDate", "must be a date, YYYY-MM-DD")
	n := utf8.RuneCountInString(req.Reason)
	check(n >= 1 && n <= 200 && strings.TrimSpace(req.Reason) != "", "reason", "must be 1 to 200 characters")
	if problems != nil {
		return saga.Refund{}, problems
	}
	return saga.Refund{RefundAuthorizationID: req.RefundAuthorizationID, ReservationID: req.ReservationID,
		Medium: saga.Medium(req.OutflowMedium), AmountCents: amount, BusinessDate: req.BusinessDate, Reason: req.Reason}, nil
}
