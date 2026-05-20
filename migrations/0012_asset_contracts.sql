-- +goose Up
ALTER TABLE rails
  ADD COLUMN IF NOT EXISTS contract_address TEXT NULL;

CREATE TABLE IF NOT EXISTS asset_contracts (
  id                          SERIAL PRIMARY KEY,
  chain_id                    INT NOT NULL REFERENCES chains(id) ON DELETE CASCADE,
  coin_id                     INT NOT NULL REFERENCES coins(id) ON DELETE CASCADE,
  contract_address            TEXT NOT NULL,
  contract_address_normalized TEXT NOT NULL,
  source                      TEXT NOT NULL,
  source_asset_id             TEXT NOT NULL DEFAULT '',
  confidence                  SMALLINT NOT NULL DEFAULT 3,
  first_seen                  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (chain_id, contract_address_normalized)
);

CREATE INDEX IF NOT EXISTS asset_contracts_coin ON asset_contracts (coin_id);

INSERT INTO coins (slug, symbol, name, external_ids) VALUES
  ('btcb', 'BTCB', 'Binance Bitcoin', '{"coingecko":"binance-bitcoin","cmc_slug":"bitcoin-bep2"}'::jsonb)
ON CONFLICT (slug) DO UPDATE
SET symbol = EXCLUDED.symbol,
    name = EXCLUDED.name,
    external_ids = coins.external_ids || EXCLUDED.external_ids;

WITH family AS (
  SELECT id FROM asset_families WHERE slug = 'btc-equivalent'
), member AS (
  SELECT id AS coin_id FROM coins WHERE slug = 'btcb'
)
INSERT INTO asset_family_members (family_id, coin_id, role, confidence, notes)
SELECT family.id, member.coin_id, 'wrapped', 3, 'Binance tokenized Bitcoin representation.'
FROM family, member
ON CONFLICT (family_id, coin_id) DO UPDATE
SET role = EXCLUDED.role,
    confidence = GREATEST(asset_family_members.confidence, EXCLUDED.confidence),
    notes = EXCLUDED.notes;

WITH contract_seed(chain_slug, coin_slug, contract_address, source, source_asset_id) AS (
  VALUES
    ('ethereum', 'wbtc', '0x2260fac5e5542a773aa44fbcfedf7c193bc2c599', 'coingecko', 'wrapped-bitcoin'),
    ('base', 'wbtc', '0x0555e30da8f98308edb960aa94c0db47230d2b9c', 'coingecko', 'wrapped-bitcoin'),
    ('bsc', 'wbtc', '0x0555e30da8f98308edb960aa94c0db47230d2b9c', 'coingecko', 'wrapped-bitcoin'),
    ('tron', 'wbtc', 'TYhWwKpw43ENFWBTGpzLHn3882f2au7SMi', 'coingecko', 'wrapped-bitcoin'),
    ('solana', 'wbtc', '5XZw2LKTyrfvfiskJ78AMpackRjPcyCif1WhUsPDuVqQ', 'coingecko', 'wrapped-bitcoin'),
    ('optimism', 'wbtc', '0x68f180fcce6836688e9084f035309e29bf0a2095', 'coingecko', 'wrapped-bitcoin'),
    ('bsc', 'btcb', '0x7130d2a12b9bcbfae4f2634d864a1ee1ce3ead9c', 'coingecko', 'binance-bitcoin'),
    ('ethereum', 'cbbtc', '0xcbb7c0000ab88b473b1f5afd9ef808440eed33bf', 'coingecko', 'coinbase-wrapped-btc'),
    ('base', 'cbbtc', '0xcbb7c0000ab88b473b1f5afd9ef808440eed33bf', 'coingecko', 'coinbase-wrapped-btc'),
    ('arbitrum', 'cbbtc', '0xcbb7c0000ab88b473b1f5afd9ef808440eed33bf', 'coingecko', 'coinbase-wrapped-btc'),
    ('solana', 'cbbtc', 'cbbtcf3aa214zXHbiAZQwf4122FBYbraNdFqgw4iMij', 'coingecko', 'coinbase-wrapped-btc'),
    ('ethereum', 'tbtc', '0x18084fba666a33d37592fa2633fd49a74dd93a88', 'coingecko', 'tbtc'),
    ('polygon', 'tbtc', '0x236aa50979d5f3de3bd1eeb40e81137f22ab794b', 'coingecko', 'tbtc'),
    ('base', 'tbtc', '0x236aa50979d5f3de3bd1eeb40e81137f22ab794b', 'coingecko', 'tbtc'),
    ('arbitrum', 'tbtc', '0x6c84a8f1c29108f47a79964b5fe888d4f4d0de40', 'coingecko', 'tbtc'),
    ('optimism', 'tbtc', '0x6c84a8f1c29108f47a79964b5fe888d4f4d0de40', 'coingecko', 'tbtc'),
    ('solana', 'tbtc', '6DNSN2BJsaPFdFFc1zP37kkeNe4Usc1Sqkzr9C9vPWcU', 'coingecko', 'tbtc')
)
INSERT INTO asset_contracts (
  chain_id, coin_id, contract_address, contract_address_normalized,
  source, source_asset_id, confidence, first_seen, last_seen
)
SELECT chains.id,
       coins.id,
       contract_seed.contract_address,
       CASE
         WHEN contract_seed.contract_address LIKE '0x%' THEN lower(contract_seed.contract_address)
         ELSE contract_seed.contract_address
       END,
       contract_seed.source,
       contract_seed.source_asset_id,
       3,
       now(),
       now()
FROM contract_seed
JOIN chains ON chains.slug = contract_seed.chain_slug
JOIN coins ON coins.slug = contract_seed.coin_slug
ON CONFLICT (chain_id, contract_address_normalized) DO UPDATE
SET coin_id = EXCLUDED.coin_id,
    contract_address = EXCLUDED.contract_address,
    source = EXCLUDED.source,
    source_asset_id = EXCLUDED.source_asset_id,
    confidence = GREATEST(asset_contracts.confidence, EXCLUDED.confidence),
    last_seen = EXCLUDED.last_seen;

-- +goose Down
DROP TABLE IF EXISTS asset_contracts;

ALTER TABLE rails
  DROP COLUMN IF EXISTS contract_address;
