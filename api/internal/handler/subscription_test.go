package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/subscription"
)

// fakeSubscriptionService is an in-memory SubscriptionService used to test
// handlers without a database or the real service/validation layer.
type fakeSubscriptionService struct {
	createFn  func(ctx context.Context, input subscription.CreateInput) (subscription.Subscription, error)
	getFn     func(ctx context.Context, id string) (subscription.Subscription, error)
	listFn    func(ctx context.Context, activeOnly bool) ([]subscription.Subscription, error)
	updateFn  func(ctx context.Context, id string, patch subscription.SubscriptionPatch) (subscription.Subscription, error)
	cancelFn  func(ctx context.Context, id string) (subscription.Subscription, error)
	summaryFn func(ctx context.Context) (subscription.Summary, error)
}

func (f *fakeSubscriptionService) Create(ctx context.Context, input subscription.CreateInput) (subscription.Subscription, error) {
	return f.createFn(ctx, input)
}

func (f *fakeSubscriptionService) Get(ctx context.Context, id string) (subscription.Subscription, error) {
	return f.getFn(ctx, id)
}

func (f *fakeSubscriptionService) List(ctx context.Context, activeOnly bool) ([]subscription.Subscription, error) {
	return f.listFn(ctx, activeOnly)
}

func (f *fakeSubscriptionService) Update(ctx context.Context, id string, patch subscription.SubscriptionPatch) (subscription.Subscription, error) {
	return f.updateFn(ctx, id, patch)
}

func (f *fakeSubscriptionService) Cancel(ctx context.Context, id string) (subscription.Subscription, error) {
	return f.cancelFn(ctx, id)
}

func (f *fakeSubscriptionService) Summary(ctx context.Context) (subscription.Summary, error) {
	return f.summaryFn(ctx)
}

func sampleSubscription() subscription.Subscription {
	t := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	return subscription.Subscription{
		ID:         "11111111-1111-1111-1111-111111111111",
		Name:       "Netflix",
		CostCents:  1599,
		Currency:   "USD",
		Cycle:      subscription.CycleMonthly,
		BillingDay: 15,
		StartDate:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Active:     true,
		CreatedAt:  t,
		UpdatedAt:  t,
	}
}

const validCreateBody = `{
	"name": "Netflix",
	"cost_cents": 1599,
	"currency": "USD",
	"cycle": "monthly",
	"billing_day": 15,
	"start_date": "2024-01-01"
}`

func TestCreateSubscription_Returns201AndBody(t *testing.T) {
	want := sampleSubscription()
	svc := &fakeSubscriptionService{
		createFn: func(ctx context.Context, input subscription.CreateInput) (subscription.Subscription, error) {
			return want, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/subscriptions", strings.NewReader(validCreateBody))
	rec := httptest.NewRecorder()

	CreateSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var body subscriptionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ID != want.ID {
		t.Errorf("ID = %q, want %q", body.ID, want.ID)
	}
	if body.Name != want.Name {
		t.Errorf("Name = %q, want %q", body.Name, want.Name)
	}
	if body.StartDate != "2024-01-01" {
		t.Errorf("StartDate = %q, want %q", body.StartDate, "2024-01-01")
	}
}

func TestCreateSubscription_ValidationErrorReturns400(t *testing.T) {
	svc := &fakeSubscriptionService{
		createFn: func(ctx context.Context, input subscription.CreateInput) (subscription.Subscription, error) {
			return subscription.Subscription{}, &subscription.ValidationError{
				Fields: map[string]string{"name": "must not be empty"},
			}
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/subscriptions", strings.NewReader(validCreateBody))
	rec := httptest.NewRecorder()

	CreateSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != errCodeValidation {
		t.Errorf("error.code = %q, want %q", body.Error.Code, errCodeValidation)
	}
	if msg, ok := body.Error.Fields["name"]; !ok || msg == "" {
		t.Errorf("error.fields[name] = %q, want a non-empty message", msg)
	}
}

func TestCreateSubscription_MalformedBodyReturns400(t *testing.T) {
	svc := &fakeSubscriptionService{
		createFn: func(ctx context.Context, input subscription.CreateInput) (subscription.Subscription, error) {
			t.Fatal("service.Create should not be called for a malformed body")
			return subscription.Subscription{}, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/subscriptions", bytes.NewReader([]byte(`{not-json`)))
	rec := httptest.NewRecorder()

	CreateSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != errCodeMalformedBody {
		t.Errorf("error.code = %q, want %q", body.Error.Code, errCodeMalformedBody)
	}
}

func TestCreateSubscription_InvalidStartDateReturns400(t *testing.T) {
	svc := &fakeSubscriptionService{
		createFn: func(ctx context.Context, input subscription.CreateInput) (subscription.Subscription, error) {
			t.Fatal("service.Create should not be called when start_date fails to parse")
			return subscription.Subscription{}, nil
		},
	}

	body := `{"name":"Netflix","cost_cents":1599,"currency":"USD","cycle":"monthly","billing_day":15,"start_date":"not-a-date"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/subscriptions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	CreateSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error.Code != errCodeValidation {
		t.Errorf("error.code = %q, want %q", resp.Error.Code, errCodeValidation)
	}
	if _, ok := resp.Error.Fields["start_date"]; !ok {
		t.Errorf("error.fields = %v, want a start_date entry", resp.Error.Fields)
	}
}

func TestGetSubscription_Returns200(t *testing.T) {
	want := sampleSubscription()
	svc := &fakeSubscriptionService{
		getFn: func(ctx context.Context, id string) (subscription.Subscription, error) {
			if id != want.ID {
				t.Fatalf("Get id = %q, want %q", id, want.ID)
			}
			return want, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/"+want.ID, nil)
	req.SetPathValue("id", want.ID)
	rec := httptest.NewRecorder()

	GetSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body subscriptionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ID != want.ID {
		t.Errorf("ID = %q, want %q", body.ID, want.ID)
	}
}

func TestGetSubscription_MissingReturns404(t *testing.T) {
	svc := &fakeSubscriptionService{
		getFn: func(ctx context.Context, id string) (subscription.Subscription, error) {
			return subscription.Subscription{}, subscription.ErrNotFound
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/missing", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()

	GetSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != errCodeNotFound {
		t.Errorf("error.code = %q, want %q", body.Error.Code, errCodeNotFound)
	}
}

func TestGetSubscription_OtherErrorReturns500(t *testing.T) {
	svc := &fakeSubscriptionService{
		getFn: func(ctx context.Context, id string) (subscription.Subscription, error) {
			return subscription.Subscription{}, errors.New("boom")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/x", nil)
	req.SetPathValue("id", "x")
	rec := httptest.NewRecorder()

	GetSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestListSubscriptions_Returns200(t *testing.T) {
	want := []subscription.Subscription{sampleSubscription(), sampleSubscription()}
	var gotActiveOnly bool
	svc := &fakeSubscriptionService{
		listFn: func(ctx context.Context, activeOnly bool) ([]subscription.Subscription, error) {
			gotActiveOnly = activeOnly
			return want, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	rec := httptest.NewRecorder()

	ListSubscriptions(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotActiveOnly != false {
		t.Errorf("activeOnly = %v, want false when ?active is absent", gotActiveOnly)
	}

	var body listSubscriptionsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Subscriptions) != len(want) {
		t.Errorf("len(subscriptions) = %d, want %d", len(body.Subscriptions), len(want))
	}
}

func TestListSubscriptions_ActiveQueryParam(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		wantActiveOnly bool
	}{
		{name: "active=true filters to active only", query: "?active=true", wantActiveOnly: true},
		{name: "active=false returns all", query: "?active=false", wantActiveOnly: false},
		{name: "absent returns all", query: "", wantActiveOnly: false},
		{name: "garbage value returns all", query: "?active=banana", wantActiveOnly: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotActiveOnly bool
			svc := &fakeSubscriptionService{
				listFn: func(ctx context.Context, activeOnly bool) ([]subscription.Subscription, error) {
					gotActiveOnly = activeOnly
					return nil, nil
				},
			}

			req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions"+tt.query, nil)
			rec := httptest.NewRecorder()

			ListSubscriptions(svc).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if gotActiveOnly != tt.wantActiveOnly {
				t.Errorf("activeOnly = %v, want %v", gotActiveOnly, tt.wantActiveOnly)
			}
		})
	}
}

func TestUpdateSubscription_Returns200(t *testing.T) {
	want := sampleSubscription()
	want.Name = "Disney+"

	var gotPatch subscription.SubscriptionPatch
	svc := &fakeSubscriptionService{
		updateFn: func(ctx context.Context, id string, patch subscription.SubscriptionPatch) (subscription.Subscription, error) {
			if id != want.ID {
				t.Fatalf("Update id = %q, want %q", id, want.ID)
			}
			gotPatch = patch
			return want, nil
		},
	}

	body := `{"name":"Disney+"}`
	req := httptest.NewRequest(http.MethodPatch, "/v1/subscriptions/"+want.ID, strings.NewReader(body))
	req.SetPathValue("id", want.ID)
	rec := httptest.NewRecorder()

	UpdateSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var respBody subscriptionResponse
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if respBody.Name != "Disney+" {
		t.Errorf("Name = %q, want %q", respBody.Name, "Disney+")
	}
	if gotPatch.Name == nil || *gotPatch.Name != "Disney+" {
		t.Errorf("service received patch.Name = %v, want pointer to %q", gotPatch.Name, "Disney+")
	}
	if gotPatch.CostCents != nil {
		t.Errorf("service received patch.CostCents = %v, want nil (absent field)", gotPatch.CostCents)
	}
}

func TestUpdateSubscription_ValidationErrorReturns400(t *testing.T) {
	svc := &fakeSubscriptionService{
		updateFn: func(ctx context.Context, id string, patch subscription.SubscriptionPatch) (subscription.Subscription, error) {
			return subscription.Subscription{}, &subscription.ValidationError{
				Fields: map[string]string{"cost_cents": "must be greater than zero"},
			}
		},
	}

	body := `{"cost_cents":0}`
	req := httptest.NewRequest(http.MethodPatch, "/v1/subscriptions/sub-1", strings.NewReader(body))
	req.SetPathValue("id", "sub-1")
	rec := httptest.NewRecorder()

	UpdateSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var respBody errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if respBody.Error.Code != errCodeValidation {
		t.Errorf("error.code = %q, want %q", respBody.Error.Code, errCodeValidation)
	}
	if _, ok := respBody.Error.Fields["cost_cents"]; !ok {
		t.Errorf("error.fields = %v, want a cost_cents entry", respBody.Error.Fields)
	}
}

func TestUpdateSubscription_MissingReturns404(t *testing.T) {
	svc := &fakeSubscriptionService{
		updateFn: func(ctx context.Context, id string, patch subscription.SubscriptionPatch) (subscription.Subscription, error) {
			return subscription.Subscription{}, subscription.ErrNotFound
		},
	}

	body := `{"name":"Disney+"}`
	req := httptest.NewRequest(http.MethodPatch, "/v1/subscriptions/missing", strings.NewReader(body))
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()

	UpdateSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	var respBody errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if respBody.Error.Code != errCodeNotFound {
		t.Errorf("error.code = %q, want %q", respBody.Error.Code, errCodeNotFound)
	}
}

func TestUpdateSubscription_MalformedBodyReturns400(t *testing.T) {
	svc := &fakeSubscriptionService{
		updateFn: func(ctx context.Context, id string, patch subscription.SubscriptionPatch) (subscription.Subscription, error) {
			t.Fatal("service.Update should not be called for a malformed body")
			return subscription.Subscription{}, nil
		},
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/subscriptions/sub-1", bytes.NewReader([]byte(`{not-json`)))
	req.SetPathValue("id", "sub-1")
	rec := httptest.NewRecorder()

	UpdateSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var respBody errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if respBody.Error.Code != errCodeMalformedBody {
		t.Errorf("error.code = %q, want %q", respBody.Error.Code, errCodeMalformedBody)
	}
}

func TestUpdateSubscription_InvalidStartDateReturns400(t *testing.T) {
	svc := &fakeSubscriptionService{
		updateFn: func(ctx context.Context, id string, patch subscription.SubscriptionPatch) (subscription.Subscription, error) {
			t.Fatal("service.Update should not be called when start_date fails to parse")
			return subscription.Subscription{}, nil
		},
	}

	body := `{"start_date":"not-a-date"}`
	req := httptest.NewRequest(http.MethodPatch, "/v1/subscriptions/sub-1", strings.NewReader(body))
	req.SetPathValue("id", "sub-1")
	rec := httptest.NewRecorder()

	UpdateSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var respBody errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := respBody.Error.Fields["start_date"]; !ok {
		t.Errorf("error.fields = %v, want a start_date entry", respBody.Error.Fields)
	}
}

func TestCancelSubscription_Returns200(t *testing.T) {
	want := sampleSubscription()
	want.Active = false
	svc := &fakeSubscriptionService{
		cancelFn: func(ctx context.Context, id string) (subscription.Subscription, error) {
			if id != want.ID {
				t.Fatalf("Cancel id = %q, want %q", id, want.ID)
			}
			return want, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/subscriptions/"+want.ID+"/cancel", nil)
	req.SetPathValue("id", want.ID)
	rec := httptest.NewRecorder()

	CancelSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var respBody subscriptionResponse
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if respBody.Active {
		t.Error("Active = true, want false after cancel")
	}
}

func TestCancelSubscription_IdempotentReturns200(t *testing.T) {
	want := sampleSubscription()
	want.Active = false
	svc := &fakeSubscriptionService{
		cancelFn: func(ctx context.Context, id string) (subscription.Subscription, error) {
			// Service-layer idempotency: already-cancelled is returned as-is.
			return want, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/subscriptions/"+want.ID+"/cancel", nil)
	req.SetPathValue("id", want.ID)
	rec := httptest.NewRecorder()

	CancelSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var respBody subscriptionResponse
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if respBody.Active {
		t.Error("Active = true, want false (idempotent re-cancel)")
	}
}

func TestCancelSubscription_MissingReturns404(t *testing.T) {
	svc := &fakeSubscriptionService{
		cancelFn: func(ctx context.Context, id string) (subscription.Subscription, error) {
			return subscription.Subscription{}, subscription.ErrNotFound
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/subscriptions/missing/cancel", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()

	CancelSubscription(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	var respBody errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if respBody.Error.Code != errCodeNotFound {
		t.Errorf("error.code = %q, want %q", respBody.Error.Code, errCodeNotFound)
	}
}

func TestListSubscriptions_ErrorReturns500(t *testing.T) {
	svc := &fakeSubscriptionService{
		listFn: func(ctx context.Context, activeOnly bool) ([]subscription.Subscription, error) {
			return nil, errors.New("boom")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	rec := httptest.NewRecorder()

	ListSubscriptions(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestSummarySubscriptions_Returns200AndExpectedShape(t *testing.T) {
	want := subscription.Summary{
		MonthlyTotalCents: 2432,
		AnnualTotalCents:  29187,
		Subscriptions: []subscription.SubscriptionSummary{
			{
				ID:              "sub-1",
				Name:            "Netflix",
				PaidToDateCents: 3198,
				NextChargeDate:  time.Date(2024, 4, 15, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	svc := &fakeSubscriptionService{
		summaryFn: func(ctx context.Context) (subscription.Summary, error) {
			return want, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/summary", nil)
	rec := httptest.NewRecorder()

	SummarySubscriptions(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body summaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.MonthlyTotalCents != want.MonthlyTotalCents {
		t.Errorf("MonthlyTotalCents = %d, want %d", body.MonthlyTotalCents, want.MonthlyTotalCents)
	}
	if body.AnnualTotalCents != want.AnnualTotalCents {
		t.Errorf("AnnualTotalCents = %d, want %d", body.AnnualTotalCents, want.AnnualTotalCents)
	}
	if len(body.Subscriptions) != 1 {
		t.Fatalf("len(subscriptions) = %d, want 1", len(body.Subscriptions))
	}
	got := body.Subscriptions[0]
	if got.ID != want.Subscriptions[0].ID {
		t.Errorf("ID = %q, want %q", got.ID, want.Subscriptions[0].ID)
	}
	if got.Name != want.Subscriptions[0].Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Subscriptions[0].Name)
	}
	if got.PaidToDateCents != want.Subscriptions[0].PaidToDateCents {
		t.Errorf("PaidToDateCents = %d, want %d", got.PaidToDateCents, want.Subscriptions[0].PaidToDateCents)
	}
	if got.NextChargeDate != "2024-04-15" {
		t.Errorf("NextChargeDate = %q, want %q", got.NextChargeDate, "2024-04-15")
	}
}

func TestSummarySubscriptions_EmptyReturnsEmptyArrayNotNull(t *testing.T) {
	svc := &fakeSubscriptionService{
		summaryFn: func(ctx context.Context) (subscription.Summary, error) {
			return subscription.Summary{}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/summary", nil)
	rec := httptest.NewRecorder()

	SummarySubscriptions(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), `"subscriptions":[]`) {
		t.Errorf("body = %s, want subscriptions to render as an empty array, not null", rec.Body.String())
	}
}

func TestSummarySubscriptions_ErrorReturns500(t *testing.T) {
	svc := &fakeSubscriptionService{
		summaryFn: func(ctx context.Context) (subscription.Summary, error) {
			return subscription.Summary{}, errors.New("boom")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/summary", nil)
	rec := httptest.NewRecorder()

	SummarySubscriptions(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}
