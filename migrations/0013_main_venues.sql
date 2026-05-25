-- +goose Up
INSERT INTO exchanges (slug, name, region) VALUES
  ('binance', 'Binance', 'Global'),
  ('bybit', 'Bybit', 'Global'),
  ('bitget', 'Bitget', 'Global'),
  ('gate', 'Gate.io', 'Global')
ON CONFLICT (slug) DO UPDATE
SET name = EXCLUDED.name,
    region = EXCLUDED.region;

INSERT INTO adapter_freshness (exchange_id)
SELECT id FROM exchanges
WHERE slug IN ('binance','bybit','bitget','gate')
ON CONFLICT (exchange_id) DO NOTHING;

-- +goose Down
UPDATE exchanges
SET name = 'Gate'
WHERE slug = 'gate';

