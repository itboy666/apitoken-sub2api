-- Durable invoice reservations and transaction claims must survive restarts,
-- plugin uninstall/reinstall and payment-order archival. Never reuse amounts.
CREATE TABLE IF NOT EXISTS sub2api_usdt_invoices (
 order_id BIGINT PRIMARY KEY,
 out_trade_no VARCHAR(64) NOT NULL UNIQUE,
 plugin_id BIGINT NOT NULL,
 wallet VARCHAR(34) NOT NULL,
 contract VARCHAR(34) NOT NULL CHECK (contract = 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t'),
 amount_micro BIGINT NOT NULL CHECK (amount_micro > 0),
 pay_cny NUMERIC(20,2) NOT NULL CHECK (pay_cny > 0),
 rate_cny NUMERIC(10,4) NOT NULL CHECK (rate_cny = 6.5),
 created_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 last_checked_at TIMESTAMPTZ,
 check_failed BOOLEAN NOT NULL DEFAULT FALSE,
 transaction_hash VARCHAR(64) UNIQUE CHECK (transaction_hash ~ '^[0-9a-f]{64}$'),
 proof JSONB,
 claimed_at TIMESTAMPTZ,
 UNIQUE (wallet, contract, amount_micro),
 CHECK (expires_at > created_at),
 CHECK ((transaction_hash IS NULL AND proof IS NULL AND claimed_at IS NULL) OR
        (transaction_hash IS NOT NULL AND proof IS NOT NULL AND claimed_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_sub2api_usdt_invoice_scan ON sub2api_usdt_invoices(last_checked_at NULLS FIRST, order_id);
