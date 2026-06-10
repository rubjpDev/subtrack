package subscription

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	minBillingDay = 1
	maxBillingDay = 28
	currencyLen   = 3
)

// Repository is the persistence seam the service depends on. It is defined
// here (consumer-side) so the service can be tested against an in-memory fake
// without any database, and so internal/postgres can implement it without the
// subscription package depending on pgx.
type Repository interface {
	Create(ctx context.Context, input CreateInput) (Subscription, error)
	GetByID(ctx context.Context, id string) (Subscription, error)
	List(ctx context.Context, activeOnly bool) ([]Subscription, error)
	Update(ctx context.Context, id string, input UpdateInput) (Subscription, error)
	SetActive(ctx context.Context, id string, active bool) (Subscription, error)
}

// Service implements subscription business rules: validation on write,
// not-found mapping on read, and computed insights (e.g. Summary) on top of a
// Repository. now is a clock seam — defaulting to time.Now — so that
// time-dependent computations (Summary) can be tested deterministically with
// an injected fixed clock.
type Service struct {
	repo Repository
	now  func() time.Time
}

// Option configures optional Service behaviour. The zero set of options
// yields the production default (a real time.Now clock).
type Option func(*Service)

// WithClock overrides the Service's clock. Intended for tests that need
// deterministic, time-dependent computations (e.g. Summary).
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		s.now = now
	}
}

// NewService builds a Service backed by the given repository. Its signature
// is intentionally unchanged from prior features so existing call sites (e.g.
// cmd/api) keep compiling; optional behaviour is layered on via opts.
func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{repo: repo, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Create validates the input and, if valid, persists a new subscription via
// the repository. Validation failures are returned as *ValidationError.
func (s *Service) Create(ctx context.Context, input CreateInput) (Subscription, error) {
	if err := validateCreateInput(input); err != nil {
		return Subscription{}, err
	}

	sub, err := s.repo.Create(ctx, input)
	if err != nil {
		return Subscription{}, fmt.Errorf("subscription: create: %w", err)
	}
	return sub, nil
}

// Get fetches a subscription by id, mapping a repository "not found" outcome
// to the domain ErrNotFound sentinel so handlers can render 404 consistently.
func (s *Service) Get(ctx context.Context, id string) (Subscription, error) {
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, fmt.Errorf("subscription: get: %w", err)
	}
	return sub, nil
}

// Update applies a partial patch to the subscription identified by id: it
// loads the existing record, merges only the caller-provided fields over it,
// validates the merged result, and persists the fully-resolved fields via the
// repository. A missing subscription yields ErrNotFound; a merged result that
// fails validation yields *ValidationError. Note: the load-then-update is not
// transactional (TOCTOU is possible under concurrent writers); that is an
// accepted tradeoff for this light-lane feature — no locking/transactions.
func (s *Service) Update(ctx context.Context, id string, patch SubscriptionPatch) (Subscription, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return Subscription{}, err
	}

	merged := mergePatch(existing, patch)

	verr := &ValidationError{}
	validateFields(merged.Name, merged.CostCents, merged.Currency, merged.Cycle, merged.BillingDay, merged.StartDate, verr)
	if verr.hasErrors() {
		return Subscription{}, verr
	}

	input := UpdateInput{
		Name:       merged.Name,
		CostCents:  merged.CostCents,
		Currency:   merged.Currency,
		Cycle:      merged.Cycle,
		BillingDay: merged.BillingDay,
		StartDate:  merged.StartDate,
	}

	sub, err := s.repo.Update(ctx, id, input)
	if err != nil {
		if isNotFound(err) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, fmt.Errorf("subscription: update: %w", err)
	}
	return sub, nil
}

// mergePatch returns a copy of existing with every non-nil field of patch
// applied over it. Absent (nil) fields leave the existing value untouched,
// which is what makes the resulting update partial.
func mergePatch(existing Subscription, patch SubscriptionPatch) Subscription {
	merged := existing

	if patch.Name != nil {
		merged.Name = *patch.Name
	}
	if patch.CostCents != nil {
		merged.CostCents = *patch.CostCents
	}
	if patch.Currency != nil {
		merged.Currency = *patch.Currency
	}
	if patch.Cycle != nil {
		merged.Cycle = *patch.Cycle
	}
	if patch.BillingDay != nil {
		merged.BillingDay = *patch.BillingDay
	}
	if patch.StartDate != nil {
		merged.StartDate = *patch.StartDate
	}

	return merged
}

// Cancel deactivates the subscription identified by id. It is idempotent at
// the service layer: if the subscription is already inactive, it is returned
// as-is with no repository write and no updated_at bump (a true no-op).
// Otherwise the repository flips active to false (which also bumps
// updated_at). A missing subscription yields ErrNotFound.
func (s *Service) Cancel(ctx context.Context, id string) (Subscription, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return Subscription{}, err
	}

	if !existing.Active {
		return existing, nil
	}

	sub, err := s.repo.SetActive(ctx, id, false)
	if err != nil {
		if isNotFound(err) {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, fmt.Errorf("subscription: cancel: %w", err)
	}
	return sub, nil
}

// List returns subscriptions, optionally filtered to active ones only.
//
// activeOnly mirrors the HTTP `?active=` query parameter contract: true means
// "active subscriptions only"; false means "all subscriptions" (the handler
// is responsible for translating the raw query value to this bool).
func (s *Service) List(ctx context.Context, activeOnly bool) ([]Subscription, error) {
	subs, err := s.repo.List(ctx, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("subscription: list: %w", err)
	}
	return subs, nil
}

// Summary computes the monthly/annual cent totals and per-subscription
// breakdown (KD-1) across active subscriptions only — cancelled subscriptions
// are excluded at the query (repo.List(activeOnly=true)) and therefore never
// reach, let alone influence, the pure computation helpers. All arithmetic
// lives in computeSummary and its helpers; this method only fetches data and
// supplies the clock.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	subs, err := s.repo.List(ctx, true)
	if err != nil {
		return Summary{}, fmt.Errorf("subscription: summary: %w", err)
	}
	return computeSummary(s.now(), subs), nil
}

// isNotFound reports whether err represents a "no such row" outcome from the
// repository. Repositories may return ErrNotFound directly or wrap it.
func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// validateCreateInput checks all create-time business rules and returns a
// *ValidationError describing every failing field, or nil if input is valid.
func validateCreateInput(input CreateInput) error {
	verr := &ValidationError{}

	validateFields(input.Name, input.CostCents, input.Currency, input.Cycle, input.BillingDay, input.StartDate, verr)

	if verr.hasErrors() {
		return verr
	}
	return nil
}

// validateFields runs every per-field business rule against the given values,
// recording failures on verr. It is the single shared validation path for both
// Create (via validateCreateInput) and Update (via the merged patch result),
// so the rules are defined exactly once.
func validateFields(name string, costCents int, currency, cycle string, billingDay int, startDate time.Time, verr *ValidationError) {
	validateName(name, verr)
	validateCostCents(costCents, verr)
	validateCurrency(currency, verr)
	validateCycle(cycle, verr)
	validateBillingDay(billingDay, verr)
	validateStartDate(startDate, verr)
}

func validateName(name string, verr *ValidationError) {
	if name == "" {
		verr.addField("name", "must not be empty")
	}
}

func validateCostCents(costCents int, verr *ValidationError) {
	if costCents <= 0 {
		verr.addField("cost_cents", "must be greater than zero")
	}
}

func validateCurrency(currency string, verr *ValidationError) {
	if len(currency) != currencyLen || !isASCIILetters(currency) {
		verr.addField("currency", "must be exactly 3 ASCII letters")
	}
}

func isASCIILetters(s string) bool {
	for _, r := range s {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func validateCycle(cycle string, verr *ValidationError) {
	if cycle != CycleMonthly && cycle != CycleYearly {
		verr.addField("cycle", "must be one of: monthly, yearly")
	}
}

func validateBillingDay(billingDay int, verr *ValidationError) {
	if billingDay < minBillingDay || billingDay > maxBillingDay {
		verr.addField("billing_day", "must be between 1 and 28")
	}
}

func validateStartDate(startDate time.Time, verr *ValidationError) {
	if startDate.IsZero() {
		verr.addField("start_date", "is required")
	}
}
