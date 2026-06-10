package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/subscription"
)

// SubscriptionService is the seam this handler depends on. It is a small,
// consumer-defined interface satisfied by *subscription.Service, which lets
// tests substitute a fake and stay DB-free.
type SubscriptionService interface {
	Create(ctx context.Context, input subscription.CreateInput) (subscription.Subscription, error)
	Get(ctx context.Context, id string) (subscription.Subscription, error)
	List(ctx context.Context, activeOnly bool) ([]subscription.Subscription, error)
	Update(ctx context.Context, id string, patch subscription.SubscriptionPatch) (subscription.Subscription, error)
	Cancel(ctx context.Context, id string) (subscription.Subscription, error)
	Summary(ctx context.Context) (subscription.Summary, error)
}

// dateLayout is the wire format for dates (YYYY-MM-DD), matching the SQL
// `date` column and keeping the JSON contract independent of time-of-day.
const dateLayout = "2006-01-02"

// createSubscriptionRequest is the explicit wire shape for POST /v1/subscriptions.
// It intentionally omits id/active/created_at/updated_at — those are
// server/DB-managed (see subscription.CreateInput).
type createSubscriptionRequest struct {
	Name       string `json:"name"`
	CostCents  int    `json:"cost_cents"`
	Currency   string `json:"currency"`
	Cycle      string `json:"cycle"`
	BillingDay int    `json:"billing_day"`
	StartDate  string `json:"start_date"`
}

// subscriptionResponse is the explicit wire shape returned for a single
// subscription. It is local to the handler so the internal domain model never
// leaks across the HTTP boundary.
type subscriptionResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CostCents  int    `json:"cost_cents"`
	Currency   string `json:"currency"`
	Cycle      string `json:"cycle"`
	BillingDay int    `json:"billing_day"`
	StartDate  string `json:"start_date"`
	Active     bool   `json:"active"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// listSubscriptionsResponse is the explicit wire shape for GET /v1/subscriptions.
type listSubscriptionsResponse struct {
	Subscriptions []subscriptionResponse `json:"subscriptions"`
}

// subscriptionSummaryResponse is the explicit per-subscription wire shape
// nested in summaryResponse. It is local to the handler so the domain model
// (subscription.SubscriptionSummary) never leaks across the HTTP boundary.
type subscriptionSummaryResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	PaidToDateCents int    `json:"paid_to_date_cents"`
	NextChargeDate  string `json:"next_charge_date"`
}

// summaryResponse is the explicit wire shape for GET /v1/subscriptions/summary.
type summaryResponse struct {
	MonthlyTotalCents int                           `json:"monthly_total_cents"`
	AnnualTotalCents  int                           `json:"annual_total_cents"`
	Subscriptions     []subscriptionSummaryResponse `json:"subscriptions"`
}

// CreateSubscription returns a handler for POST /v1/subscriptions. It decodes
// the request, delegates validation+persistence to the service, and maps the
// outcome to 201 + body, 400 (malformed body / validation), or a wrapped 500.
func CreateSubscription(svc SubscriptionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createSubscriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeMalformedBodyError(w)
			return
		}

		input, err := toCreateInput(req)
		if err != nil {
			writeValidationError(w, map[string]string{"start_date": "must be a valid date in YYYY-MM-DD format"})
			return
		}

		sub, err := svc.Create(r.Context(), input)
		if err != nil {
			writeCreateError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, toSubscriptionResponse(sub))
	}
}

// writeCreateError maps a Create error to its HTTP representation: validation
// failures become a structured 400, anything else a 500.
func writeCreateError(w http.ResponseWriter, err error) {
	var verr *subscription.ValidationError
	if errors.As(err, &verr) {
		writeValidationError(w, verr.Fields)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "failed to create subscription", nil)
}

// toCreateInput converts the wire request to the service's CreateInput,
// parsing the date field. A parse failure is reported as a validation error
// on start_date so the client gets consistent, structured feedback.
func toCreateInput(req createSubscriptionRequest) (subscription.CreateInput, error) {
	startDate, err := time.Parse(dateLayout, req.StartDate)
	if err != nil {
		return subscription.CreateInput{}, err
	}

	return subscription.CreateInput{
		Name:       req.Name,
		CostCents:  req.CostCents,
		Currency:   req.Currency,
		Cycle:      req.Cycle,
		BillingDay: req.BillingDay,
		StartDate:  startDate,
	}, nil
}

// GetSubscription returns a handler for GET /v1/subscriptions/{id}. It maps
// outcomes to 200 + body or 404.
func GetSubscription(svc SubscriptionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		sub, err := svc.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, subscription.ErrNotFound) {
				writeNotFoundError(w, "subscription not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to fetch subscription", nil)
			return
		}

		writeJSON(w, http.StatusOK, toSubscriptionResponse(sub))
	}
}

// updateSubscriptionRequest is the explicit wire shape for
// PATCH /v1/subscriptions/{id}. Every field is a pointer so absence ("not in
// the JSON body") is distinguishable from a provided zero value, which is
// what makes the update partial. id/active/created_at/updated_at are
// intentionally absent — they are not client-settable here (active changes
// only via the cancel endpoint).
type updateSubscriptionRequest struct {
	Name       *string `json:"name"`
	CostCents  *int    `json:"cost_cents"`
	Currency   *string `json:"currency"`
	Cycle      *string `json:"cycle"`
	BillingDay *int    `json:"billing_day"`
	StartDate  *string `json:"start_date"`
}

// UpdateSubscription returns a handler for PATCH /v1/subscriptions/{id}. It
// decodes a partial request, delegates merge+validation+persistence to the
// service, and maps the outcome to 200 + body, 400 (malformed body /
// validation), 404, or a wrapped 500.
func UpdateSubscription(svc SubscriptionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		var req updateSubscriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeMalformedBodyError(w)
			return
		}

		patch, err := toSubscriptionPatch(req)
		if err != nil {
			writeValidationError(w, map[string]string{"start_date": "must be a valid date in YYYY-MM-DD format"})
			return
		}

		sub, err := svc.Update(r.Context(), id, patch)
		if err != nil {
			writeUpdateError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, toSubscriptionResponse(sub))
	}
}

// writeUpdateError maps an Update error to its HTTP representation:
// validation failures become a structured 400, not-found a 404, anything else
// a 500.
func writeUpdateError(w http.ResponseWriter, err error) {
	var verr *subscription.ValidationError
	if errors.As(err, &verr) {
		writeValidationError(w, verr.Fields)
		return
	}
	if errors.Is(err, subscription.ErrNotFound) {
		writeNotFoundError(w, "subscription not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "failed to update subscription", nil)
}

// toSubscriptionPatch converts the wire request to the service's
// SubscriptionPatch, parsing the date field when provided. A parse failure is
// reported as a validation error on start_date so the client gets consistent,
// structured feedback (mirroring toCreateInput).
func toSubscriptionPatch(req updateSubscriptionRequest) (subscription.SubscriptionPatch, error) {
	patch := subscription.SubscriptionPatch{
		Name:       req.Name,
		CostCents:  req.CostCents,
		Currency:   req.Currency,
		Cycle:      req.Cycle,
		BillingDay: req.BillingDay,
	}

	if req.StartDate != nil {
		startDate, err := time.Parse(dateLayout, *req.StartDate)
		if err != nil {
			return subscription.SubscriptionPatch{}, err
		}
		patch.StartDate = &startDate
	}

	return patch, nil
}

// CancelSubscription returns a handler for POST /v1/subscriptions/{id}/cancel.
// It takes no request body and is idempotent: cancelling an already-cancelled
// subscription returns 200 with the unchanged resource (a no-op). Maps
// outcomes to 200 + body, 404, or a wrapped 500.
func CancelSubscription(svc SubscriptionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		sub, err := svc.Cancel(r.Context(), id)
		if err != nil {
			if errors.Is(err, subscription.ErrNotFound) {
				writeNotFoundError(w, "subscription not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to cancel subscription", nil)
			return
		}

		writeJSON(w, http.StatusOK, toSubscriptionResponse(sub))
	}
}

// ListSubscriptions returns a handler for GET /v1/subscriptions?active=.
//
// Query contract: `active=true` restricts the list to active subscriptions
// only; an absent or any other value returns all subscriptions. This mirrors
// subscription.Service.List's activeOnly parameter directly.
func ListSubscriptions(svc SubscriptionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		activeOnly := r.URL.Query().Get("active") == "true"

		subs, err := svc.List(r.Context(), activeOnly)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list subscriptions", nil)
			return
		}

		writeJSON(w, http.StatusOK, toListResponse(subs))
	}
}

// SummarySubscriptions returns a handler for GET /v1/subscriptions/summary.
// All computation (totals, per-subscription breakdown) happens in the service
// layer (KD-1) — this handler only invokes it and marshals the result to its
// explicit wire shape. Maps outcomes to 200 + body or a wrapped 500.
func SummarySubscriptions(svc SubscriptionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := svc.Summary(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to compute subscription summary", nil)
			return
		}

		writeJSON(w, http.StatusOK, toSummaryResponse(summary))
	}
}

// toSummaryResponse maps a domain Summary to its wire shape, formatting dates
// as strings (dateLayout) so the JSON contract is explicit and independent of
// the domain model's Go types. The per-subscription slice is always non-nil
// (an empty summary renders as `"subscriptions": []`, never null).
func toSummaryResponse(summary subscription.Summary) summaryResponse {
	items := make([]subscriptionSummaryResponse, 0, len(summary.Subscriptions))
	for _, item := range summary.Subscriptions {
		items = append(items, subscriptionSummaryResponse{
			ID:              item.ID,
			Name:            item.Name,
			PaidToDateCents: item.PaidToDateCents,
			NextChargeDate:  item.NextChargeDate.Format(dateLayout),
		})
	}

	return summaryResponse{
		MonthlyTotalCents: summary.MonthlyTotalCents,
		AnnualTotalCents:  summary.AnnualTotalCents,
		Subscriptions:     items,
	}
}

// toSubscriptionResponse maps a domain Subscription to its wire shape,
// formatting dates/timestamps as strings so the JSON contract is explicit and
// independent of the domain model's Go types.
func toSubscriptionResponse(sub subscription.Subscription) subscriptionResponse {
	return subscriptionResponse{
		ID:         sub.ID,
		Name:       sub.Name,
		CostCents:  sub.CostCents,
		Currency:   sub.Currency,
		Cycle:      sub.Cycle,
		BillingDay: sub.BillingDay,
		StartDate:  sub.StartDate.Format(dateLayout),
		Active:     sub.Active,
		CreatedAt:  sub.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  sub.UpdatedAt.Format(time.RFC3339),
	}
}

func toListResponse(subs []subscription.Subscription) listSubscriptionsResponse {
	items := make([]subscriptionResponse, 0, len(subs))
	for _, sub := range subs {
		items = append(items, toSubscriptionResponse(sub))
	}
	return listSubscriptionsResponse{Subscriptions: items}
}

// writeJSON encodes body as JSON with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}
