-- Preserve invoice history and transaction-hash uniqueness while allowing
-- amount reuse only after the original payment interval has ended.
CREATE TABLE IF NOT EXISTS sub2api_usdt_amount_reservations (
 wallet VARCHAR(34) NOT NULL,
 contract VARCHAR(34) NOT NULL CHECK (contract = 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t'),
 amount_micro BIGINT NOT NULL CHECK (amount_micro > 0),
 reserved_until TIMESTAMPTZ NOT NULL,
 PRIMARY KEY (wallet, contract, amount_micro)
);
INSERT INTO sub2api_usdt_amount_reservations(wallet,contract,amount_micro,reserved_until)
 SELECT wallet,contract,amount_micro,MAX(expires_at) FROM sub2api_usdt_invoices
 GROUP BY wallet,contract,amount_micro
 ON CONFLICT(wallet,contract,amount_micro) DO UPDATE
 SET reserved_until=GREATEST(sub2api_usdt_amount_reservations.reserved_until,EXCLUDED.reserved_until);
ALTER TABLE sub2api_usdt_invoices DROP CONSTRAINT IF EXISTS sub2api_usdt_invoices_wallet_contract_amount_micro_key;
CREATE INDEX IF NOT EXISTS idx_sub2api_usdt_invoice_amount_history ON sub2api_usdt_invoices(wallet,contract,amount_micro,created_at);
