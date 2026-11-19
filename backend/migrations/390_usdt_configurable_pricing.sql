-- Keep existing invoice rates and amounts immutable. New orders snapshot the
-- independently configured rate; old hosts must not process these new rates.
ALTER TABLE sub2api_usdt_invoices
    DROP CONSTRAINT IF EXISTS sub2api_usdt_invoices_rate_cny_check;
ALTER TABLE sub2api_usdt_invoices
    ADD CONSTRAINT sub2api_usdt_invoices_rate_cny_check
    CHECK (rate_cny >= 0.0001 AND rate_cny <= 100000);
