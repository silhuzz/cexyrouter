-- +goose Up
CREATE TABLE asset_families (
  id          SERIAL PRIMARY KEY,
  slug        TEXT UNIQUE NOT NULL,
  symbol      TEXT NOT NULL,
  name        TEXT NOT NULL,
  notes       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE asset_family_members (
  family_id   INT NOT NULL REFERENCES asset_families(id) ON DELETE CASCADE,
  coin_id     INT NOT NULL REFERENCES coins(id) ON DELETE CASCADE,
  role        TEXT NOT NULL DEFAULT 'wrapped' CHECK (role IN ('native','wrapped','staked','bridged')),
  confidence  SMALLINT NOT NULL DEFAULT 1,
  notes       TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (family_id, coin_id)
);

CREATE INDEX asset_family_members_coin ON asset_family_members (coin_id, family_id);

INSERT INTO coins (slug, symbol, name, external_ids) VALUES
  ('btc', 'BTC', 'Bitcoin', '{"coingecko":"bitcoin"}'::jsonb),
  ('wbtc', 'WBTC', 'Wrapped Bitcoin', '{"coingecko":"wrapped-bitcoin"}'::jsonb),
  ('tbtc', 'TBTC', 'tBTC', '{"coingecko":"tbtc"}'::jsonb),
  ('cbbtc', 'CBBTC', 'Coinbase Wrapped BTC', '{"coingecko":"coinbase-wrapped-btc"}'::jsonb),
  ('eth', 'ETH', 'Ethereum', '{"coingecko":"ethereum"}'::jsonb),
  ('weth', 'WETH', 'Wrapped Ether', '{"coingecko":"weth"}'::jsonb)
ON CONFLICT (slug) DO UPDATE
SET external_ids = coins.external_ids || EXCLUDED.external_ids,
    name = CASE
      WHEN coins.name = coins.symbol OR coins.name = '' THEN EXCLUDED.name
      ELSE coins.name
    END;

INSERT INTO asset_families (slug, symbol, name, notes) VALUES
  ('btc-equivalent', 'BTC', 'Bitcoin-equivalent assets', 'Native BTC plus wrapped/tokenized BTC representations such as WBTC, tBTC, and cbBTC. These are not the same ticker and must be shown as destination assets.'),
  ('eth-equivalent', 'ETH', 'Ethereum-equivalent assets', 'Native ETH plus wrapped ETH representations.')
ON CONFLICT (slug) DO UPDATE
SET symbol = EXCLUDED.symbol,
    name = EXCLUDED.name,
    notes = EXCLUDED.notes;

WITH family AS (
  SELECT id FROM asset_families WHERE slug = 'btc-equivalent'
), members(slug, role, notes) AS (
  VALUES
    ('btc', 'native', 'Native Bitcoin on the Bitcoin network.'),
    ('wbtc', 'wrapped', 'Wrapped Bitcoin, commonly used on Ethereum and other smart-contract chains.'),
    ('tbtc', 'wrapped', 'Threshold/tBTC tokenized Bitcoin representation.'),
    ('cbbtc', 'wrapped', 'Coinbase wrapped Bitcoin representation.')
)
INSERT INTO asset_family_members (family_id, coin_id, role, confidence, notes)
SELECT family.id, coins.id, members.role, 3, members.notes
FROM family
JOIN members ON TRUE
JOIN coins ON coins.slug = members.slug
ON CONFLICT (family_id, coin_id) DO UPDATE
SET role = EXCLUDED.role,
    confidence = GREATEST(asset_family_members.confidence, EXCLUDED.confidence),
    notes = EXCLUDED.notes;

WITH family AS (
  SELECT id FROM asset_families WHERE slug = 'eth-equivalent'
), members(slug, role, notes) AS (
  VALUES
    ('eth', 'native', 'Native ETH.'),
    ('weth', 'wrapped', 'Wrapped Ether ERC-20 representation.')
)
INSERT INTO asset_family_members (family_id, coin_id, role, confidence, notes)
SELECT family.id, coins.id, members.role, 3, members.notes
FROM family
JOIN members ON TRUE
JOIN coins ON coins.slug = members.slug
ON CONFLICT (family_id, coin_id) DO UPDATE
SET role = EXCLUDED.role,
    confidence = GREATEST(asset_family_members.confidence, EXCLUDED.confidence),
    notes = EXCLUDED.notes;

-- +goose Down
DROP TABLE IF EXISTS asset_family_members;
DROP TABLE IF EXISTS asset_families;
