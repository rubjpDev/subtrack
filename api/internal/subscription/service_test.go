package subscription

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepository is an in-memory Repository used to test the service without
// any database dependency.
type fakeRepository struct {
	createFn    func(ctx context.Context, input CreateInput) (Subscription, error)
	getFn       func(ctx context.Context, id string) (Subscription, error)
	listFn      func(ctx context.Context, activeOnly bool) ([]Subscription, error)
	updateFn    func(ctx context.Context, id string, input UpdateInput) (Subscription, error)
	setActiveFn func(ctx context.Context, id string, active bool) (Subscription, error)

	// setActiveCalls counts SetActive invocations so idempotency tests can
	// assert that a no-op Cancel performs NO repository write.
	setActiveCalls int
}

func (f *fakeRepository) Create(ctx context.Context, input CreateInput) (Subscription, error) {
	return f.createFn(ctx, input)
}

func (f *fakeRepository) GetByID(ctx context.Context, id string) (Subscription, error) {
	return f.getFn(ctx, id)
}

func (f *fakeRepository) List(ctx context.Context, activeOnly bool) ([]Subscription, error) {
	return f.listFn(ctx, activeOnly)
}

func (f *fakeRepository) Update(ctx context.Context, id string, input UpdateInput) (Subscription, error) {
	return f.updateFn(ctx, id, input)
}

func (f *fakeRepository) SetActive(ctx context.Context, id string, active bool) (Subscription, error) {
	f.setActiveCalls++
	return f.setActiveFn(ctx, id, active)
}

func validCreateInput() CreateInput {
	return CreateInput{
		Name:       "Netflix",
		CostCents:  1599,
		Currency:   "USD",
		Cycle:      CycleMonthly,
		BillingDay: 15,
		StartDate:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestValidateCreateInput(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(in *CreateInput)
		wantField   string
		wantInvalid bool
	}{
		{name: "valid input passes", mutate: func(in *CreateInput) {}, wantInvalid: false},
		{name: "empty name", mutate: func(in *CreateInput) { in.Name = "" }, wantField: "name", wantInvalid: true},
		{name: "zero cost", mutate: func(in *CreateInput) { in.CostCents = 0 }, wantField: "cost_cents", wantInvalid: true},
		{name: "negative cost", mutate: func(in *CreateInput) { in.CostCents = -5 }, wantField: "cost_cents", wantInvalid: true},
		{name: "currency too short", mutate: func(in *CreateInput) { in.Currency = "US" }, wantField: "currency", wantInvalid: true},
		{name: "currency too long", mutate: func(in *CreateInput) { in.Currency = "USDX" }, wantField: "currency", wantInvalid: true},
		{name: "currency non-letters", mutate: func(in *CreateInput) { in.Currency = "U5D" }, wantField: "currency", wantInvalid: true},
		{name: "invalid cycle", mutate: func(in *CreateInput) { in.Cycle = "weekly" }, wantField: "cycle", wantInvalid: true},
		{name: "billing day too low", mutate: func(in *CreateInput) { in.BillingDay = 0 }, wantField: "billing_day", wantInvalid: true},
		{name: "billing day too high", mutate: func(in *CreateInput) { in.BillingDay = 29 }, wantField: "billing_day", wantInvalid: true},
		{name: "missing start date", mutate: func(in *CreateInput) { in.StartDate = time.Time{} }, wantField: "start_date", wantInvalid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validCreateInput()
			tt.mutate(&input)

			err := validateCreateInput(input)

			if !tt.wantInvalid {
				if err != nil {
					t.Fatalf("validateCreateInput() error = %v, want nil", err)
				}
				return
			}

			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("validateCreateInput() error type = %T, want *ValidationError", err)
			}
			if _, ok := verr.Fields[tt.wantField]; !ok {
				t.Errorf("ValidationError.Fields = %v, want key %q", verr.Fields, tt.wantField)
			}
		})
	}
}

func TestService_Create_ValidationError(t *testing.T) {
	repo := &fakeRepository{
		createFn: func(ctx context.Context, input CreateInput) (Subscription, error) {
			t.Fatal("repo.Create should not be called when validation fails")
			return Subscription{}, nil
		},
	}
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), CreateInput{})

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Create() error type = %T, want *ValidationError", err)
	}
}

func TestService_Create_DelegatesToRepository(t *testing.T) {
	want := Subscription{ID: "sub-1", Name: "Netflix"}
	var gotInput CreateInput
	repo := &fakeRepository{
		createFn: func(ctx context.Context, input CreateInput) (Subscription, error) {
			gotInput = input
			return want, nil
		},
	}
	svc := NewService(repo)

	input := validCreateInput()
	got, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if got != want {
		t.Errorf("Create() = %+v, want %+v", got, want)
	}
	if gotInput != input {
		t.Errorf("repo received input = %+v, want %+v", gotInput, input)
	}
}

func TestService_Create_WrapsRepositoryError(t *testing.T) {
	repoErr := errors.New("connection refused")
	repo := &fakeRepository{
		createFn: func(ctx context.Context, input CreateInput) (Subscription, error) {
			return Subscription{}, repoErr
		},
	}
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), validCreateInput())
	if err == nil {
		t.Fatal("Create() error = nil, want non-nil")
	}
	if !errors.Is(err, repoErr) {
		t.Errorf("Create() error = %v, want it to wrap %v", err, repoErr)
	}
}

func TestService_Get_Found(t *testing.T) {
	want := Subscription{ID: "sub-1", Name: "Netflix"}
	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			if id != "sub-1" {
				t.Fatalf("GetByID id = %q, want %q", id, "sub-1")
			}
			return want, nil
		},
	}
	svc := NewService(repo)

	got, err := svc.Get(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got != want {
		t.Errorf("Get() = %+v, want %+v", got, want)
	}
}

func TestService_Get_NotFound(t *testing.T) {
	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			return Subscription{}, ErrNotFound
		},
	}
	svc := NewService(repo)

	_, err := svc.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestService_Get_NotFound_WrappedByRepo(t *testing.T) {
	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			return Subscription{}, errors.Join(ErrNotFound, errors.New("pgx: no rows"))
		},
	}
	svc := NewService(repo)

	_, err := svc.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() error = %v, want it to be ErrNotFound", err)
	}
}

func TestService_Get_OtherError(t *testing.T) {
	repoErr := errors.New("connection refused")
	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			return Subscription{}, repoErr
		},
	}
	svc := NewService(repo)

	_, err := svc.Get(context.Background(), "sub-1")
	if errors.Is(err, ErrNotFound) {
		t.Fatal("Get() error = ErrNotFound, want a wrapped repo error instead")
	}
	if !errors.Is(err, repoErr) {
		t.Errorf("Get() error = %v, want it to wrap %v", err, repoErr)
	}
}

func TestService_List(t *testing.T) {
	tests := []struct {
		name       string
		activeOnly bool
	}{
		{name: "all", activeOnly: false},
		{name: "active only", activeOnly: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := []Subscription{{ID: "sub-1"}, {ID: "sub-2"}}
			var gotActiveOnly bool
			repo := &fakeRepository{
				listFn: func(ctx context.Context, activeOnly bool) ([]Subscription, error) {
					gotActiveOnly = activeOnly
					return want, nil
				},
			}
			svc := NewService(repo)

			got, err := svc.List(context.Background(), tt.activeOnly)
			if err != nil {
				t.Fatalf("List() error = %v, want nil", err)
			}
			if len(got) != len(want) {
				t.Fatalf("List() returned %d items, want %d", len(got), len(want))
			}
			if gotActiveOnly != tt.activeOnly {
				t.Errorf("repo received activeOnly = %v, want %v", gotActiveOnly, tt.activeOnly)
			}
		})
	}
}

func sampleSubscription(id string) Subscription {
	t := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	return Subscription{
		ID:         id,
		Name:       "Netflix",
		CostCents:  1599,
		Currency:   "USD",
		Cycle:      CycleMonthly,
		BillingDay: 15,
		StartDate:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Active:     true,
		CreatedAt:  t,
		UpdatedAt:  t,
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestService_Update_MergesAndPersistsOnlyProvidedFields(t *testing.T) {
	existing := sampleSubscription("sub-1")
	want := existing
	want.Name = "Disney+"
	want.CostCents = 1299

	var gotInput UpdateInput
	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			return existing, nil
		},
		updateFn: func(ctx context.Context, id string, input UpdateInput) (Subscription, error) {
			gotInput = input
			return want, nil
		},
	}
	svc := NewService(repo)

	patch := SubscriptionPatch{Name: strPtr("Disney+"), CostCents: intPtr(1299)}
	got, err := svc.Update(context.Background(), "sub-1", patch)
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if got != want {
		t.Errorf("Update() = %+v, want %+v", got, want)
	}

	// Provided fields changed...
	if gotInput.Name != "Disney+" {
		t.Errorf("repo received Name = %q, want %q", gotInput.Name, "Disney+")
	}
	if gotInput.CostCents != 1299 {
		t.Errorf("repo received CostCents = %d, want %d", gotInput.CostCents, 1299)
	}
	// ...others preserved from the loaded record.
	if gotInput.Currency != existing.Currency {
		t.Errorf("repo received Currency = %q, want preserved %q", gotInput.Currency, existing.Currency)
	}
	if gotInput.Cycle != existing.Cycle {
		t.Errorf("repo received Cycle = %q, want preserved %q", gotInput.Cycle, existing.Cycle)
	}
	if gotInput.BillingDay != existing.BillingDay {
		t.Errorf("repo received BillingDay = %d, want preserved %d", gotInput.BillingDay, existing.BillingDay)
	}
	if !gotInput.StartDate.Equal(existing.StartDate) {
		t.Errorf("repo received StartDate = %v, want preserved %v", gotInput.StartDate, existing.StartDate)
	}
}

func TestService_Update_ValidationErrorOnMergedResult(t *testing.T) {
	existing := sampleSubscription("sub-1")
	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			return existing, nil
		},
		updateFn: func(ctx context.Context, id string, input UpdateInput) (Subscription, error) {
			t.Fatal("repo.Update should not be called when the merged result fails validation")
			return Subscription{}, nil
		},
	}
	svc := NewService(repo)

	patch := SubscriptionPatch{CostCents: intPtr(0)}
	_, err := svc.Update(context.Background(), "sub-1", patch)

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Update() error type = %T, want *ValidationError", err)
	}
	if _, ok := verr.Fields["cost_cents"]; !ok {
		t.Errorf("ValidationError.Fields = %v, want key %q", verr.Fields, "cost_cents")
	}
}

func TestService_Update_NotFound(t *testing.T) {
	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			return Subscription{}, ErrNotFound
		},
		updateFn: func(ctx context.Context, id string, input UpdateInput) (Subscription, error) {
			t.Fatal("repo.Update should not be called when the subscription does not exist")
			return Subscription{}, nil
		},
	}
	svc := NewService(repo)

	_, err := svc.Update(context.Background(), "missing", SubscriptionPatch{Name: strPtr("X")})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestService_Cancel_ActiveBecomesInactive(t *testing.T) {
	existing := sampleSubscription("sub-1")
	want := existing
	want.Active = false
	want.UpdatedAt = existing.UpdatedAt.Add(time.Hour)

	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			return existing, nil
		},
		setActiveFn: func(ctx context.Context, id string, active bool) (Subscription, error) {
			if active != false {
				t.Errorf("SetActive active = %v, want false", active)
			}
			return want, nil
		},
	}
	svc := NewService(repo)

	got, err := svc.Cancel(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("Cancel() error = %v, want nil", err)
	}
	if got != want {
		t.Errorf("Cancel() = %+v, want %+v", got, want)
	}
	if repo.setActiveCalls != 1 {
		t.Errorf("SetActive call count = %d, want 1", repo.setActiveCalls)
	}
}

func TestService_Cancel_AlreadyCancelledIsNoOp(t *testing.T) {
	existing := sampleSubscription("sub-1")
	existing.Active = false

	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			return existing, nil
		},
		setActiveFn: func(ctx context.Context, id string, active bool) (Subscription, error) {
			t.Fatal("repo.SetActive should not be called when the subscription is already inactive")
			return Subscription{}, nil
		},
	}
	svc := NewService(repo)

	got, err := svc.Cancel(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("Cancel() error = %v, want nil", err)
	}
	if got != existing {
		t.Errorf("Cancel() = %+v, want unchanged %+v", got, existing)
	}
	if got.UpdatedAt != existing.UpdatedAt {
		t.Errorf("Cancel() UpdatedAt = %v, want unchanged %v (no-op must not bump it)", got.UpdatedAt, existing.UpdatedAt)
	}
	if repo.setActiveCalls != 0 {
		t.Errorf("SetActive call count = %d, want 0 (no write on no-op)", repo.setActiveCalls)
	}
}

func TestService_Cancel_NotFound(t *testing.T) {
	repo := &fakeRepository{
		getFn: func(ctx context.Context, id string) (Subscription, error) {
			return Subscription{}, ErrNotFound
		},
		setActiveFn: func(ctx context.Context, id string, active bool) (Subscription, error) {
			t.Fatal("repo.SetActive should not be called when the subscription does not exist")
			return Subscription{}, nil
		},
	}
	svc := NewService(repo)

	_, err := svc.Cancel(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Cancel() error = %v, want ErrNotFound", err)
	}
	if repo.setActiveCalls != 0 {
		t.Errorf("SetActive call count = %d, want 0", repo.setActiveCalls)
	}
}

func TestService_List_WrapsRepositoryError(t *testing.T) {
	repoErr := errors.New("connection refused")
	repo := &fakeRepository{
		listFn: func(ctx context.Context, activeOnly bool) ([]Subscription, error) {
			return nil, repoErr
		},
	}
	svc := NewService(repo)

	_, err := svc.List(context.Background(), false)
	if !errors.Is(err, repoErr) {
		t.Errorf("List() error = %v, want it to wrap %v", err, repoErr)
	}
}
