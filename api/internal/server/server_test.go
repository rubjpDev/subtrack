package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/handler"
	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/subscription"
)

// stubSubscriptionService is a no-op handler.SubscriptionService used only to
// satisfy server.New's wiring in tests that don't exercise subscription routes.
type stubSubscriptionService struct{}

func (stubSubscriptionService) Create(ctx context.Context, input subscription.CreateInput) (subscription.Subscription, error) {
	return subscription.Subscription{}, nil
}

func (stubSubscriptionService) Get(ctx context.Context, id string) (subscription.Subscription, error) {
	return subscription.Subscription{}, nil
}

func (stubSubscriptionService) List(ctx context.Context, activeOnly bool) ([]subscription.Subscription, error) {
	return nil, nil
}

func (stubSubscriptionService) Update(ctx context.Context, id string, patch subscription.SubscriptionPatch) (subscription.Subscription, error) {
	return subscription.Subscription{}, nil
}

func (stubSubscriptionService) Cancel(ctx context.Context, id string) (subscription.Subscription, error) {
	return subscription.Subscription{}, nil
}

func (stubSubscriptionService) Summary(ctx context.Context) (subscription.Summary, error) {
	return subscription.Summary{}, nil
}

// testAPIKey is the shared secret used to build servers under test. Requests
// that should authenticate successfully set this on X-API-Key.
const testAPIKey = "test-server-api-key"

// TestNew_SummaryRouteCoexistsWithIDRoute verifies that the literal
// "/v1/subscriptions/summary" segment is routed to SummarySubscriptions and
// does not get swallowed by the "/v1/subscriptions/{id}" wildcard pattern —
// Go 1.22+ ServeMux treats literal segments as more specific than wildcards,
// so both routes coexist without conflict. Both requests authenticate with a
// valid X-API-Key, since /v1/* routes now require it.
func TestNew_SummaryRouteCoexistsWithIDRoute(t *testing.T) {
	srv := New(":0", handler.PingerFunc(func(ctx context.Context) error { return nil }), stubSubscriptionService{}, testAPIKey)

	summaryReq := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/summary", nil)
	summaryReq.Header.Set("X-API-Key", testAPIKey)
	summaryRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/subscriptions/summary status = %d, want %d; body = %s", summaryRec.Code, http.StatusOK, summaryRec.Body.String())
	}
	if !strings.Contains(summaryRec.Body.String(), `"monthly_total_cents"`) {
		t.Errorf("GET /v1/subscriptions/summary body = %s, want a summary shape (monthly_total_cents)", summaryRec.Body.String())
	}

	idReq := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/some-id", nil)
	idReq.Header.Set("X-API-Key", testAPIKey)
	idRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(idRec, idReq)
	if idRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/subscriptions/{id} status = %d, want %d; body = %s", idRec.Code, http.StatusOK, idRec.Body.String())
	}
	if strings.Contains(idRec.Body.String(), `"monthly_total_cents"`) {
		t.Errorf("GET /v1/subscriptions/{id} was routed to the summary handler; body = %s", idRec.Body.String())
	}
}

func TestNew_HealthzRouteReturnsOK(t *testing.T) {
	healthyPinger := handler.PingerFunc(func(ctx context.Context) error { return nil })
	srv := New(":0", healthyPinger, stubSubscriptionService{}, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestNew_HealthzExemptFromAPIKey re-asserts the /healthz exemption explicitly:
// the request carries no X-API-Key at all and must still succeed, proving
// /healthz is reachable for orchestration/monitoring without a key.
func TestNew_HealthzExemptFromAPIKey(t *testing.T) {
	healthyPinger := handler.PingerFunc(func(ctx context.Context) error { return nil })
	srv := New(":0", healthyPinger, stubSubscriptionService{}, testAPIKey)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (healthz must be exempt from API key auth)", rec.Code, http.StatusOK)
	}
}

// TestNew_V1RouteRequiresAPIKey checks the auth boundary on a representative
// /v1 route: rejected with 401 when no key is sent, and reachable (200) when
// the correct X-API-Key is sent.
func TestNew_V1RouteRequiresAPIKey(t *testing.T) {
	healthyPinger := handler.PingerFunc(func(ctx context.Context) error { return nil })
	srv := New(":0", healthyPinger, stubSubscriptionService{}, testAPIKey)

	noKeyReq := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/summary", nil)
	noKeyRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(noKeyRec, noKeyReq)
	if noKeyRec.Code != http.StatusUnauthorized {
		t.Fatalf("without X-API-Key: status = %d, want %d; body = %s", noKeyRec.Code, http.StatusUnauthorized, noKeyRec.Body.String())
	}

	validKeyReq := httptest.NewRequest(http.MethodGet, "/v1/subscriptions/summary", nil)
	validKeyReq.Header.Set("X-API-Key", testAPIKey)
	validKeyRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(validKeyRec, validKeyReq)
	if validKeyRec.Code != http.StatusOK {
		t.Fatalf("with valid X-API-Key: status = %d, want %d; body = %s", validKeyRec.Code, http.StatusOK, validKeyRec.Body.String())
	}
}
