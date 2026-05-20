-- +goose Up
INSERT INTO exchanges (slug, name, region) VALUES
  ('bitget', 'Bitget', 'Global'),
  ('kucoin', 'KuCoin', 'Global'),
  ('gate', 'Gate', 'Global')
ON CONFLICT (slug) DO UPDATE
SET name = EXCLUDED.name,
    region = EXCLUDED.region;

INSERT INTO adapter_freshness (exchange_id)
SELECT id FROM exchanges
WHERE slug IN ('bitget','kucoin','gate')
ON CONFLICT (exchange_id) DO NOTHING;

-- +goose Down
DELETE FROM adapter_freshness
WHERE exchange_id IN (SELECT id FROM exchanges WHERE slug IN ('bitget','kucoin','gate'));

DELETE FROM exchanges
WHERE slug IN ('bitget','kucoin','gate');
