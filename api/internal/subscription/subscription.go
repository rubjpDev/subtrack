// Package subscription contains the subscription domain: the model, the
// service that validates and orchestrates against a repository, and the
// domain errors returned across that boundary. No HTTP or SQL concerns live
// here — those belong to internal/handler and internal/postgres respectively.
package subscription

import "time"

// Cycle enumerates the supported billing cycles.
const (
	CycleMonthly = "monthly"
	CycleYearly  = "yearly"
)

// Subscription is the domain model for a tracked subscription. Money is kept
// as integer cents — never floats — per project convention (KD-3).
type Subscription struct {
	ID         string
	Name       string
	CostCents  int
	Currency   string
	Cycle      string
	BillingDay int
	StartDate  time.Time
	Active     bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateInput carries the fields a caller may supply when creating a
// subscription. Active, ID, CreatedAt and UpdatedAt are server/DB-managed and
// therefore intentionally absent here.
type CreateInput struct {
	Name       string
	CostCents  int
	Currency   string
	Cycle      string
	BillingDay int
	StartDate  time.Time
}

// UpdateInput carries the resolved mutable fields the repository persists for
// a PATCH. It is the full set of mutable columns — the service is responsible
// for merging any partial caller input over the existing record and producing
// this fully-resolved value before calling the repository (the repository
// itself performs a static full-field UPDATE).
type UpdateInput struct {
	Name       string
	CostCents  int
	Currency   string
	Cycle      string
	BillingDay int
	StartDate  time.Time
}

// SubscriptionPatch carries the fields a caller may supply on a PATCH. Pointer
// fields distinguish "absent" (nil — leave unchanged) from "provided" (non-nil
// — replace), which is what makes the update partial. Active/ID/CreatedAt/
// UpdatedAt are not client-settable here — active changes only via Cancel.
type SubscriptionPatch struct {
	Name       *string
	CostCents  *int
	Currency   *string
	Cycle      *string
	BillingDay *int
	StartDate  *time.Time
}
