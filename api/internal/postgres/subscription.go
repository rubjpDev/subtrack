// Package postgres contains pgx-backed implementations of the repository
// interfaces declared by domain packages (e.g. subscription.Repository). It
// owns SQL and row-mapping concerns; it never contains business rules or
// schema definitions (schema lives in /migrations — KD-4).
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/subscription"
)

// SubscriptionRepository implements subscription.Repository against a pgx
// connection pool, using parameterized queries only.
type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

// NewSubscriptionRepository builds a repository backed by the given pool.
func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool}
}

const insertSubscriptionSQL = `
INSERT INTO subscriptions (name, cost_cents, currency, cycle, billing_day, start_date)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, name, cost_cents, currency, cycle, billing_day, start_date, active, created_at, updated_at
`

// Create inserts a new subscription row, letting the database assign id,
// active (default true), created_at and updated_at, and returns the
// resulting row mapped to the domain model.
func (r *SubscriptionRepository) Create(ctx context.Context, input subscription.CreateInput) (subscription.Subscription, error) {
	row := r.pool.QueryRow(ctx, insertSubscriptionSQL,
		input.Name,
		input.CostCents,
		input.Currency,
		input.Cycle,
		input.BillingDay,
		input.StartDate,
	)

	sub, err := scanSubscription(row)
	if err != nil {
		return subscription.Subscription{}, fmt.Errorf("postgres: create subscription: %w", err)
	}
	return sub, nil
}

const selectSubscriptionByIDSQL = `
SELECT id, name, cost_cents, currency, cycle, billing_day, start_date, active, created_at, updated_at
FROM subscriptions
WHERE id = $1
`

// GetByID fetches a subscription by id. When no row matches, it returns
// subscription.ErrNotFound so the service can map it without depending on pgx.
func (r *SubscriptionRepository) GetByID(ctx context.Context, id string) (subscription.Subscription, error) {
	row := r.pool.QueryRow(ctx, selectSubscriptionByIDSQL, id)

	sub, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return subscription.Subscription{}, subscription.ErrNotFound
		}
		return subscription.Subscription{}, fmt.Errorf("postgres: get subscription: %w", err)
	}
	return sub, nil
}

const (
	selectAllSubscriptionsSQL = `
SELECT id, name, cost_cents, currency, cycle, billing_day, start_date, active, created_at, updated_at
FROM subscriptions
ORDER BY created_at DESC
`
	selectActiveSubscriptionsSQL = `
SELECT id, name, cost_cents, currency, cycle, billing_day, start_date, active, created_at, updated_at
FROM subscriptions
WHERE active = true
ORDER BY created_at DESC
`
)

// List returns subscriptions ordered by most recently created first,
// optionally restricted to active rows only.
func (r *SubscriptionRepository) List(ctx context.Context, activeOnly bool) ([]subscription.Subscription, error) {
	query := selectAllSubscriptionsSQL
	if activeOnly {
		query = selectActiveSubscriptionsSQL
	}

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres: list subscriptions: %w", err)
	}
	defer rows.Close()

	subs, err := scanSubscriptions(rows)
	if err != nil {
		return nil, fmt.Errorf("postgres: list subscriptions: %w", err)
	}
	return subs, nil
}

const updateSubscriptionSQL = `
UPDATE subscriptions
SET name = $2, cost_cents = $3, currency = $4, cycle = $5, billing_day = $6, start_date = $7, updated_at = now()
WHERE id = $1
RETURNING id, name, cost_cents, currency, cycle, billing_day, start_date, active, created_at, updated_at
`

// Update performs a static full-field UPDATE of the mutable columns (name,
// cost_cents, currency, cycle, billing_day, start_date) for the row matching
// id, also bumping updated_at, and returns the resulting row mapped to the
// domain model. The service is responsible for resolving the full set of
// fields to write (merging any partial patch beforehand) — this repository
// method knows nothing about partial semantics. When no row matches, it
// returns subscription.ErrNotFound so the service can map it without
// depending on pgx.
func (r *SubscriptionRepository) Update(ctx context.Context, id string, input subscription.UpdateInput) (subscription.Subscription, error) {
	row := r.pool.QueryRow(ctx, updateSubscriptionSQL,
		id,
		input.Name,
		input.CostCents,
		input.Currency,
		input.Cycle,
		input.BillingDay,
		input.StartDate,
	)

	sub, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return subscription.Subscription{}, subscription.ErrNotFound
		}
		return subscription.Subscription{}, fmt.Errorf("postgres: update subscription: %w", err)
	}
	return sub, nil
}

const setActiveSubscriptionSQL = `
UPDATE subscriptions
SET active = $2, updated_at = now()
WHERE id = $1
RETURNING id, name, cost_cents, currency, cycle, billing_day, start_date, active, created_at, updated_at
`

// SetActive is a thin primitive that flips the active flag for the row
// matching id, bumps updated_at, and returns the resulting row mapped to the
// domain model. Idempotency (e.g. no-op when already inactive) is a service
// concern — this method always performs the write. When no row matches, it
// returns subscription.ErrNotFound so the service can map it without
// depending on pgx.
func (r *SubscriptionRepository) SetActive(ctx context.Context, id string, active bool) (subscription.Subscription, error) {
	row := r.pool.QueryRow(ctx, setActiveSubscriptionSQL, id, active)

	sub, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return subscription.Subscription{}, subscription.ErrNotFound
		}
		return subscription.Subscription{}, fmt.Errorf("postgres: set active subscription: %w", err)
	}
	return sub, nil
}

// rowScanner abstracts the subset of pgx.Row/pgx.Rows used for mapping a
// single row to a domain Subscription, so scanSubscription works for both
// QueryRow results and Rows iteration.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanSubscription maps one database row to the domain model.
func scanSubscription(row rowScanner) (subscription.Subscription, error) {
	var sub subscription.Subscription

	err := row.Scan(
		&sub.ID,
		&sub.Name,
		&sub.CostCents,
		&sub.Currency,
		&sub.Cycle,
		&sub.BillingDay,
		&sub.StartDate,
		&sub.Active,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		return subscription.Subscription{}, err
	}
	return sub, nil
}

// scanSubscriptions maps every remaining row in rows to domain models.
func scanSubscriptions(rows pgx.Rows) ([]subscription.Subscription, error) {
	subs := make([]subscription.Subscription, 0)

	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return subs, nil
}
