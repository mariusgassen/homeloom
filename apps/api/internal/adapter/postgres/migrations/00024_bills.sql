-- +goose Up
-- A shared reminder can be a bill: ticking it off books an expense of bill_cents in bill_category,
-- paid by bill_paid_by (NULL: whoever ticks it off).
ALTER TABLE entries
    ADD COLUMN bill_cents    bigint CHECK (bill_cents > 0),
    ADD COLUMN bill_category text,
    ADD COLUMN bill_paid_by  uuid REFERENCES users (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE entries DROP COLUMN bill_cents, DROP COLUMN bill_category, DROP COLUMN bill_paid_by;
