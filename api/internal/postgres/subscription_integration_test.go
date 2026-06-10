//go:build integration

// This file requires a live Postgres reachable via DATABASE_URL (or
// POSTGRES_* parts, see internal/config) with migrations applied. Run with:
//
//	go test -tags=integration ./internal/postgres/...
//
// It is excluded from the default `go test ./...` gate so the suite stays
// DB-free and deterministic.
package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/subscription"
)

func TestSubscriptionRepository_CreateGetList(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping() error = %v", err)
	}

	repo := NewSubscriptionRepository(pool)

	input := subscription.CreateInput{
		Name:       "Integration Test Sub",
		CostCents:  999,
		Currency:   "USD",
		Cycle:      subscription.CycleMonthly,
		BillingDay: 10,
		StartDate:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	created, err := repo.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create() returned empty ID")
	}
	if !created.Active {
		t.Error("Create() returned Active = false, want true (DB default)")
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != input.Name {
		t.Errorf("GetByID() Name = %q, want %q", got.Name, input.Name)
	}

	_, err = repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
	if err != subscription.ErrNotFound {
		t.Errorf("GetByID(missing) error = %v, want subscription.ErrNotFound", err)
	}

	all, err := repo.List(ctx, false)
	if err != nil {
		t.Fatalf("List(false) error = %v", err)
	}
	if len(all) == 0 {
		t.Error("List(false) returned no rows, want at least the created one")
	}

	activeOnly, err := repo.List(ctx, true)
	if err != nil {
		t.Fatalf("List(true) error = %v", err)
	}
	for _, s := range activeOnly {
		if !s.Active {
			t.Errorf("List(true) returned inactive subscription %q", s.ID)
		}
	}
}

func TestSubscriptionRepository_UpdateAndSetActive(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping() error = %v", err)
	}

	repo := NewSubscriptionRepository(pool)

	created, err := repo.Create(ctx, subscription.CreateInput{
		Name:       "Update/Cancel Integration Sub",
		CostCents:  500,
		Currency:   "USD",
		Cycle:      subscription.CycleMonthly,
		BillingDay: 5,
		StartDate:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update bumps updated_at and persists the new field values.
	time.Sleep(10 * time.Millisecond) // ensure now() advances past created_at
	updated, err := repo.Update(ctx, created.ID, subscription.UpdateInput{
		Name:       "Renamed Sub",
		CostCents:  750,
		Currency:   "EUR",
		Cycle:      subscription.CycleYearly,
		BillingDay: 10,
		StartDate:  time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "Renamed Sub" || updated.CostCents != 750 || updated.Currency != "EUR" {
		t.Errorf("Update() did not persist new field values: %+v", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("Update() UpdatedAt = %v, want it to be after %v", updated.UpdatedAt, created.UpdatedAt)
	}

	_, err = repo.Update(ctx, "00000000-0000-0000-0000-000000000000", subscription.UpdateInput{
		Name:       "X",
		CostCents:  100,
		Currency:   "USD",
		Cycle:      subscription.CycleMonthly,
		BillingDay: 1,
		StartDate:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != subscription.ErrNotFound {
		t.Errorf("Update(missing) error = %v, want subscription.ErrNotFound", err)
	}

	// SetActive bumps updated_at and flips the flag.
	time.Sleep(10 * time.Millisecond)
	cancelled, err := repo.SetActive(ctx, created.ID, false)
	if err != nil {
		t.Fatalf("SetActive() error = %v", err)
	}
	if cancelled.Active {
		t.Error("SetActive(false) returned Active = true, want false")
	}
	if !cancelled.UpdatedAt.After(updated.UpdatedAt) {
		t.Errorf("SetActive() UpdatedAt = %v, want it to be after %v", cancelled.UpdatedAt, updated.UpdatedAt)
	}

	_, err = repo.SetActive(ctx, "00000000-0000-0000-0000-000000000000", false)
	if err != subscription.ErrNotFound {
		t.Errorf("SetActive(missing) error = %v, want subscription.ErrNotFound", err)
	}
}
