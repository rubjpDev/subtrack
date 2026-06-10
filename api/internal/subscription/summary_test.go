package subscription

import (
	"testing"
	"time"

	"context"
	"errors"
)

// fixedNow is a stable reference "today" used across the pure-computation
// tests: 2024-03-15 (a Friday), so day-of-month boundary cases are easy to
// reason about (15 == billing day; 14/16 surround it).
var fixedNow = time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC)

func TestNextChargeDate(t *testing.T) {
	tests := []struct {
		name       string
		now        time.Time
		billingDay int
		want       time.Time
	}{
		{
			name:       "today before billing day -> this month",
			now:        time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC),
			billingDay: 15,
			want:       time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "today equals billing day -> today",
			now:        time.Date(2024, 3, 15, 23, 59, 0, 0, time.UTC),
			billingDay: 15,
			want:       time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "today after billing day -> next month",
			now:        time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC),
			billingDay: 15,
			want:       time.Date(2024, 4, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "december rolls over into next january",
			now:        time.Date(2024, 12, 20, 0, 0, 0, 0, time.UTC),
			billingDay: 5,
			want:       time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextChargeDate(tt.now, tt.billingDay)
			if !got.Equal(tt.want) {
				t.Errorf("nextChargeDate(%v, %d) = %v, want %v", tt.now, tt.billingDay, got, tt.want)
			}
		})
	}
}

func TestMonthlyAndAnnualAmountCents(t *testing.T) {
	monthly := Subscription{Cycle: CycleMonthly, CostCents: 1599}
	yearly := Subscription{Cycle: CycleYearly, CostCents: 9999}

	if got, want := monthlyAmountCents(monthly), 1599; got != want {
		t.Errorf("monthlyAmountCents(monthly) = %d, want %d", got, want)
	}
	if got, want := annualAmountCents(monthly), 1599*12; got != want {
		t.Errorf("annualAmountCents(monthly) = %d, want %d", got, want)
	}

	// 9999 / 12 = 833 (Go integer division truncates toward zero) — this is
	// the intentional truncation documented on monthlyAmountCents.
	if got, want := monthlyAmountCents(yearly), 833; got != want {
		t.Errorf("monthlyAmountCents(yearly) = %d, want %d (truncated)", got, want)
	}
	if got, want := annualAmountCents(yearly), 9999; got != want {
		t.Errorf("annualAmountCents(yearly) = %d, want %d", got, want)
	}
}

func TestPaidToDateCents_Monthly(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		startDate time.Time
		costCents int
		want      int
	}{
		{
			name:      "exact months elapsed",
			now:       time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			startDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			costCents: 1000,
			want:      2 * 1000, // Jan->Feb, Feb->Mar = 2 completed months
		},
		{
			name:      "partial month dropped when today.Day < start.Day",
			now:       time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC),
			startDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			costCents: 1000,
			want:      1 * 1000, // only Jan->Feb counts; Feb->Mar not yet complete
		},
		{
			name:      "start date in the future yields zero",
			now:       time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			startDate: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			costCents: 1000,
			want:      0,
		},
		{
			name:      "start date today yields zero",
			now:       time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			startDate: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			costCents: 1000,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := Subscription{Cycle: CycleMonthly, CostCents: tt.costCents, StartDate: tt.startDate}
			if got := paidToDateCents(tt.now, sub); got != tt.want {
				t.Errorf("paidToDateCents() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPaidToDateCents_Yearly(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		startDate time.Time
		costCents int
		want      int
	}{
		{
			name:      "anniversary already reached this year -> full years",
			now:       time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			startDate: time.Date(2021, 3, 15, 0, 0, 0, 0, time.UTC),
			costCents: 12000,
			want:      3 * 12000, // 2021->2022, 2022->2023, 2023->2024
		},
		{
			name:      "anniversary not yet reached this year -> years-1",
			now:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			startDate: time.Date(2021, 3, 15, 0, 0, 0, 0, time.UTC),
			costCents: 12000,
			want:      2 * 12000, // 2024 anniversary (Mar 15) not yet reached
		},
		{
			name:      "start date in the future yields zero",
			now:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			startDate: time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC),
			costCents: 12000,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := Subscription{Cycle: CycleYearly, CostCents: tt.costCents, StartDate: tt.startDate}
			if got := paidToDateCents(tt.now, sub); got != tt.want {
				t.Errorf("paidToDateCents() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestComputeSummary_TotalsAggregation(t *testing.T) {
	subs := []Subscription{
		{ID: "sub-1", Name: "Netflix", Cycle: CycleMonthly, CostCents: 1599, BillingDay: 15, StartDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
		{ID: "sub-2", Name: "Cloud Storage", Cycle: CycleYearly, CostCents: 9999, BillingDay: 1, StartDate: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	got := computeSummary(fixedNow, subs)

	wantMonthlyTotal := 1599 + (9999 / 12) // 1599 + 833 = 2432
	wantAnnualTotal := (1599 * 12) + 9999  // 19188 + 9999 = 29187 (per-native-cycle, NOT monthlyTotal*12)

	if got.MonthlyTotalCents != wantMonthlyTotal {
		t.Errorf("MonthlyTotalCents = %d, want %d", got.MonthlyTotalCents, wantMonthlyTotal)
	}
	if got.AnnualTotalCents != wantAnnualTotal {
		t.Errorf("AnnualTotalCents = %d, want %d", got.AnnualTotalCents, wantAnnualTotal)
	}
	// Sanity: annual_total must NOT equal monthly_total*12 (that would
	// compound the yearly truncation — the very thing we deliberately avoid).
	if got.AnnualTotalCents == got.MonthlyTotalCents*12 {
		t.Errorf("AnnualTotalCents unexpectedly equals MonthlyTotalCents*12 = %d; it must be computed per native cycle", got.MonthlyTotalCents*12)
	}
	if len(got.Subscriptions) != len(subs) {
		t.Fatalf("Subscriptions len = %d, want %d", len(got.Subscriptions), len(subs))
	}
}

func TestComputeSummary_EmptyYieldsEmptySliceNotNil(t *testing.T) {
	got := computeSummary(fixedNow, nil)

	if got.Subscriptions == nil {
		t.Error("Subscriptions = nil, want non-nil empty slice")
	}
	if len(got.Subscriptions) != 0 {
		t.Errorf("Subscriptions len = %d, want 0", len(got.Subscriptions))
	}
	if got.MonthlyTotalCents != 0 || got.AnnualTotalCents != 0 {
		t.Errorf("totals = (%d, %d), want (0, 0)", got.MonthlyTotalCents, got.AnnualTotalCents)
	}
}

// TestService_Summary_ExcludesCancelledAndQueriesActiveOnly is a
// Service-level test using the fake repository: it mirrors the DB contract
// (List(activeOnly=true) returns only active rows) and asserts that a
// cancelled subscription contributes nothing to totals and is absent from the
// per-subscription list — and that Summary actually requests activeOnly=true.
func TestService_Summary_ExcludesCancelledAndQueriesActiveOnly(t *testing.T) {
	activeSub := Subscription{
		ID: "sub-active", Name: "Netflix", Cycle: CycleMonthly, CostCents: 1599,
		BillingDay: 15, StartDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Active: true,
	}
	// A cancelled subscription would never be returned by
	// List(activeOnly=true) in production (filtered at the query); the fake
	// mirrors that contract by returning only the active one.
	var gotActiveOnly bool
	repo := &fakeRepository{
		listFn: func(ctx context.Context, activeOnly bool) ([]Subscription, error) {
			gotActiveOnly = activeOnly
			return []Subscription{activeSub}, nil
		},
	}
	svc := NewService(repo, WithClock(func() time.Time { return fixedNow }))

	got, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary() error = %v, want nil", err)
	}
	if !gotActiveOnly {
		t.Error("Summary() did not call List with activeOnly == true")
	}
	if len(got.Subscriptions) != 1 || got.Subscriptions[0].ID != activeSub.ID {
		t.Errorf("Subscriptions = %+v, want exactly [%q]", got.Subscriptions, activeSub.ID)
	}
	wantMonthlyTotal := monthlyAmountCents(activeSub)
	if got.MonthlyTotalCents != wantMonthlyTotal {
		t.Errorf("MonthlyTotalCents = %d, want %d (cancelled sub must contribute nothing)", got.MonthlyTotalCents, wantMonthlyTotal)
	}
}

func TestService_Summary_WrapsRepositoryError(t *testing.T) {
	repoErr := errors.New("connection refused")
	repo := &fakeRepository{
		listFn: func(ctx context.Context, activeOnly bool) ([]Subscription, error) {
			return nil, repoErr
		},
	}
	svc := NewService(repo, WithClock(func() time.Time { return fixedNow }))

	_, err := svc.Summary(context.Background())
	if !errors.Is(err, repoErr) {
		t.Errorf("Summary() error = %v, want it to wrap %v", err, repoErr)
	}
}
