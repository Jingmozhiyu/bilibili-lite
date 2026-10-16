ALTER TABLE products ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN year INTEGER NOT NULL DEFAULT 0;
CREATE INDEX orders_payment_reconcile ON orders(reconciled_at) WHERE payment_state='processing';
