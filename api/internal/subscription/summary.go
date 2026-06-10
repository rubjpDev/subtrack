package subscription

import "time"

// monthsPerYear is used to normalize yearly costs onto a monthly basis (and
// vice versa) when aggregating totals across mixed billing cycles.
const monthsPerYear = 12

// Summary is the computed insight (KD-1) returned by Service.Summary: the
// monthly/annual cent totals across all active subscriptions plus a per-
// subscription breakdown. All money stays integer cents (KD-3).
type Summary struct {
	MonthlyTotalCents int
	AnnualTotalCents  int
	Subscriptions     []SubscriptionSummary
}

// SubscriptionSummary is the per-subscription slice of a Summary: how much has
// been paid to date (completed billing cycles only, no proration) and when the
// next charge falls.
type SubscriptionSummary struct {
	ID              string
	Name            string
	PaidToDateCents int
	NextChargeDate  time.Time
}

// computeSummary derives a Summary for the given subscriptions as of now. It
// is a pure function — the caller (Service.Summary) is responsible for
// ensuring subs is already filtered to active-only records (the single source
// of truth is the repo.List(activeOnly=true) query; this function does not
// re-filter).
func computeSummary(now time.Time, subs []Subscription) Summary {
	summary := Summary{
		Subscriptions: make([]SubscriptionSummary, 0, len(subs)),
	}

	for _, sub := range subs {
		summary.MonthlyTotalCents += monthlyAmountCents(sub)
		summary.AnnualTotalCents += annualAmountCents(sub)

		summary.Subscriptions = append(summary.Subscriptions, SubscriptionSummary{
			ID:              sub.ID,
			Name:            sub.Name,
			PaidToDateCents: paidToDateCents(now, sub),
			NextChargeDate:  nextChargeDate(now, sub.BillingDay),
		})
	}

	return summary
}

// monthlyAmountCents normalizes sub's cost onto a monthly basis: a monthly
// subscription contributes its cost as-is; a yearly subscription contributes
// cost_cents/12. This is integer division that truncates toward zero (e.g.
// 9999/12 = 833) — intentional, since money stays integer cents (KD-3) and we
// deliberately do not prorate or introduce floats.
func monthlyAmountCents(sub Subscription) int {
	if sub.Cycle == CycleYearly {
		return sub.CostCents / monthsPerYear
	}
	return sub.CostCents
}

// annualAmountCents normalizes sub's cost onto an annual basis: a yearly
// subscription contributes its cost as-is; a monthly subscription contributes
// cost_cents*12.
func annualAmountCents(sub Subscription) int {
	if sub.Cycle == CycleMonthly {
		return sub.CostCents * monthsPerYear
	}
	return sub.CostCents
}

// nextChargeDate returns the smallest date D (compared by calendar date,
// ignoring time-of-day) such that D.Day() == billingDay and D >= today:
//   - if today.Day() == billingDay, D is today;
//   - if today.Day() < billingDay, D is this month;
//   - if today.Day() > billingDay, D is next month (rolling the year over in
//     December).
//
// billingDay is always 1..28, so every month has that day — no end-of-month
// handling is required. This rule is applied UNIFORMLY for both monthly and
// yearly cycles, per the literal spec — yearly subscriptions do not get
// anniversary-based next-charge logic here.
func nextChargeDate(now time.Time, billingDay int) time.Time {
	year, month, day := now.Date()
	loc := now.Location()

	if day <= billingDay {
		return time.Date(year, month, billingDay, 0, 0, 0, 0, loc)
	}

	month++
	if month > time.December {
		month = time.January
		year++
	}
	return time.Date(year, month, billingDay, 0, 0, 0, 0, loc)
}

// paidToDateCents returns the total amount paid for completed billing cycles
// since sub.StartDate, with NO proration of partial periods: completed cycle
// count times cost_cents. A start date in the future (or today, which has zero
// elapsed cycles) yields 0.
func paidToDateCents(now time.Time, sub Subscription) int {
	if sub.Cycle == CycleYearly {
		return completedYears(now, sub.StartDate) * sub.CostCents
	}
	return completedMonths(now, sub.StartDate) * sub.CostCents
}

// completedMonths returns the number of whole months elapsed between start
// and now, using component (year/month/day) arithmetic rather than AddDate
// loops. A month is "complete" only once now's day-of-month has reached
// start's day-of-month; the result floors at 0 (a future start date yields 0).
func completedMonths(now, start time.Time) int {
	months := (now.Year()-start.Year())*monthsPerYear + (int(now.Month()) - int(start.Month()))
	if now.Day() < start.Day() {
		months--
	}
	if months < 0 {
		return 0
	}
	return months
}

// completedYears returns the number of whole years elapsed between start and
// now, using component (year/month/day) arithmetic. A year is "complete" only
// once now's (month, day) has reached start's (month, day) — i.e. the
// anniversary has occurred this year; otherwise the most recent completed
// anniversary was last year. The result floors at 0.
func completedYears(now, start time.Time) int {
	years := now.Year() - start.Year()
	if monthDayBefore(now, start) {
		years--
	}
	if years < 0 {
		return 0
	}
	return years
}

// monthDayBefore reports whether a's (month, day) comes strictly before b's
// (month, day), ignoring year — used to detect "this year's anniversary has
// not yet occurred".
func monthDayBefore(a, b time.Time) bool {
	if a.Month() != b.Month() {
		return a.Month() < b.Month()
	}
	return a.Day() < b.Day()
}
