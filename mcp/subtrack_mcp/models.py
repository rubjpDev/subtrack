from datetime import date, datetime
from typing import Literal
from pydantic import BaseModel, Field

class Subscription(BaseModel):
    id: str
    name: str
    cost_cents: int
    currency: str
    cycle: str
    billing_day: int
    start_date: date
    active: bool
    created_at: datetime
    updated_at: datetime

class SubscriptionLine(BaseModel):
    id: str
    name: str
    paid_to_date_cents: int
    next_charge_date: date

class Summary(BaseModel):
    monthly_total_cents: int
    annual_total_cents: int
    subscriptions: list[SubscriptionLine]

class NewSubscription(BaseModel):
    name: str = Field(min_length=1)
    cost_cents: int = Field(gt=0)
    currency: str = Field(min_length=3, max_length=3)
    cycle: Literal["monthly", "yearly"]
    billing_day: int = Field(ge=1, le=28)
    start_date: date

class SubscriptionUpdate(BaseModel):
    name: str | None = Field(default = None, min_length=1)
    cost_cents: int | None = Field(default=None, gt=0)
    currency: str | None = Field(default=None, min_length=3, max_length=3)
    cycle: Literal["monthly", "yearly"] | None = None
    billing_day: int | None = Field(default=None, ge=1, le=28)