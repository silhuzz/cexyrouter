-- +goose Up
INSERT INTO coins (slug, symbol, name, external_ids) VALUES
  ('usdt', 'USDT', 'Tether USD', '{}'),
  ('usdc', 'USDC', 'USD Coin', '{}'),
  ('btc', 'BTC', 'Bitcoin', '{}'),
  ('eth', 'ETH', 'Ether', '{}'),
  ('xrp', 'XRP', 'XRP', '{}'),
  ('ada', 'ADA', 'Cardano', '{}'),
  ('dot', 'DOT', 'Polkadot', '{}'),
  ('matic', 'MATIC', 'Polygon', '{}')
ON CONFLICT (slug) DO UPDATE
SET symbol = EXCLUDED.symbol,
    name = EXCLUDED.name;

INSERT INTO chains (slug, symbol, name, evm_chain_id) VALUES
  ('ethereum', 'ETH', 'Ethereum', 1),
  ('bsc', 'BSC', 'BNB Smart Chain', 56),
  ('tron', 'TRX', 'TRON', NULL),
  ('solana', 'SOL', 'Solana', NULL),
  ('polygon', 'POL', 'Polygon', 137),
  ('arbitrum', 'ARB', 'Arbitrum One', 42161),
  ('optimism', 'OP', 'Optimism', 10),
  ('base', 'BASE', 'Base', 8453),
  ('bitcoin', 'BTC', 'Bitcoin', NULL),
  ('ripple', 'XRP', 'XRP Ledger', NULL),
  ('cardano', 'ADA', 'Cardano', NULL),
  ('polkadot', 'DOT', 'Polkadot', NULL)
ON CONFLICT (slug) DO UPDATE
SET symbol = EXCLUDED.symbol,
    name = EXCLUDED.name,
    evm_chain_id = EXCLUDED.evm_chain_id;

WITH exchange_rows AS (
  SELECT id AS exchange_id FROM exchanges
), coin_rows AS (
  SELECT id AS coin_id, symbol FROM coins
  WHERE slug IN ('usdt','usdc','btc','eth','xrp','ada','dot','matic')
)
INSERT INTO coin_aliases (exchange_id, raw_symbol, raw_name, coin_id, confidence, first_seen, last_seen)
SELECT exchange_id, symbol, '', coin_id, 3, now(), now()
FROM exchange_rows CROSS JOIN coin_rows
ON CONFLICT (exchange_id, raw_symbol, raw_name) DO UPDATE
SET coin_id = EXCLUDED.coin_id,
    confidence = GREATEST(coin_aliases.confidence, EXCLUDED.confidence),
    last_seen = EXCLUDED.last_seen;

WITH alias_seed(raw_symbol, raw_name, raw_network_id, chain_slug) AS (
  VALUES
    ('ERC20', 'Ethereum', '', 'ethereum'),
    ('ERC-20', 'Ethereum', '', 'ethereum'),
    ('ETH', 'Ethereum', '', 'ethereum'),
    ('Ethereum', 'Ethereum', '', 'ethereum'),
    ('TRC20', 'TRON', '', 'tron'),
    ('TRC-20', 'TRON', '', 'tron'),
    ('TRX', 'TRON', '', 'tron'),
    ('TRON', 'TRON', '', 'tron'),
    ('BEP20', 'BNB Smart Chain', '', 'bsc'),
    ('BEP-20', 'BNB Smart Chain', '', 'bsc'),
    ('BSC', 'BNB Smart Chain', '', 'bsc'),
    ('SOL', 'Solana', '', 'solana'),
    ('SOLANA', 'Solana', '', 'solana'),
    ('MATIC', 'Polygon', '', 'polygon'),
    ('POLYGON', 'Polygon', '', 'polygon'),
    ('ARBITRUM', 'Arbitrum One', '', 'arbitrum'),
    ('ARB', 'Arbitrum One', '', 'arbitrum'),
    ('OPTIMISM', 'Optimism', '', 'optimism'),
    ('OP', 'Optimism', '', 'optimism'),
    ('BASE', 'Base', '', 'base'),
    ('BTC', 'Bitcoin', '', 'bitcoin'),
    ('BITCOIN', 'Bitcoin', '', 'bitcoin'),
    ('XRP', 'XRP Ledger', '', 'ripple'),
    ('ADA', 'Cardano', '', 'cardano'),
    ('DOT', 'Polkadot', '', 'polkadot'),
    ('USDT-ERC20', 'Ethereum', '', 'ethereum'),
    ('USDT-TRC20', 'TRON', '', 'tron'),
    ('USDT-BEP20', 'BNB Smart Chain', '', 'bsc'),
    ('USDC-ERC20', 'Ethereum', '', 'ethereum'),
    ('USDC-SOL', 'Solana', '', 'solana')
), exchange_rows AS (
  SELECT id AS exchange_id FROM exchanges
), chain_rows AS (
  SELECT id AS chain_id, slug FROM chains
)
INSERT INTO chain_aliases (exchange_id, raw_symbol, raw_name, raw_network_id, chain_id, confidence, first_seen, last_seen)
SELECT e.exchange_id, a.raw_symbol, a.raw_name, a.raw_network_id, c.chain_id, 3, now(), now()
FROM alias_seed a
JOIN chain_rows c ON c.slug = a.chain_slug
CROSS JOIN exchange_rows e
ON CONFLICT (exchange_id, raw_symbol, raw_name, raw_network_id) DO UPDATE
SET chain_id = EXCLUDED.chain_id,
    confidence = GREATEST(chain_aliases.confidence, EXCLUDED.confidence),
    last_seen = EXCLUDED.last_seen;

-- +goose Down
DELETE FROM chain_aliases
WHERE confidence = 3
  AND exchange_id IN (SELECT id FROM exchanges WHERE slug IN ('binance','bybit','okx','bithumb','upbit'));

DELETE FROM coin_aliases
WHERE confidence = 3
  AND exchange_id IN (SELECT id FROM exchanges WHERE slug IN ('binance','bybit','okx','bithumb','upbit'));

DELETE FROM chains
WHERE slug IN ('ethereum','bsc','tron','solana','polygon','arbitrum','optimism','base','bitcoin','ripple','cardano','polkadot');

DELETE FROM coins
WHERE slug IN ('usdt','usdc','btc','eth','xrp','ada','dot','matic');
