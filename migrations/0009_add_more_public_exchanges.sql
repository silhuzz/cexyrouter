-- +goose Up
INSERT INTO exchanges (slug, name, region) VALUES
  ('htx', 'HTX', 'Global'),
  ('coinex', 'CoinEx', 'Global'),
  ('whitebit', 'WhiteBIT', 'Global'),
  ('bitmart', 'BitMart', 'Global')
ON CONFLICT (slug) DO UPDATE
SET name = EXCLUDED.name,
    region = EXCLUDED.region;

INSERT INTO adapter_freshness (exchange_id)
SELECT id FROM exchanges
WHERE slug IN ('htx','coinex','whitebit','bitmart')
ON CONFLICT (exchange_id) DO NOTHING;

-- +goose Down
DELETE FROM adapter_freshness
WHERE exchange_id IN (SELECT id FROM exchanges WHERE slug IN ('htx','coinex','whitebit','bitmart'));

DELETE FROM exchanges
WHERE slug IN ('htx','coinex','whitebit','bitmart');
