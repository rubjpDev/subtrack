CREATE TABLE subscriptions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    cost_cents  int NOT NULL CHECK (cost_cents > 0),
    currency    char(3) NOT NULL,
    cycle       text NOT NULL CHECK (cycle IN ('monthly', 'yearly')),
    billing_day int NOT NULL CHECK (billing_day BETWEEN 1 AND 28),
    start_date  date NOT NULL,
    active      bool NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
