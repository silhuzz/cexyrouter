-- +goose Up
WITH canonical_chains(slug, symbol, name, evm_chain_id) AS (
  VALUES
    ('avax', 'AVAX', 'Avalanche C-Chain', 43114),
    ('base', 'BASE', 'Base', 8453),
    ('arbitrum', 'ARB', 'Arbitrum One', 42161),
    ('polygon', 'POL', 'Polygon', 137),
    ('optimism', 'OP', 'Optimism', 10),
    ('linea', 'LINEA', 'Linea', 59144),
    ('blast', 'BLAST', 'Blast', 81457),
    ('zksyncera', 'ZKSYNC', 'zkSync Era', 324)
)
INSERT INTO chains (slug, symbol, name, evm_chain_id)
SELECT slug, symbol, name, evm_chain_id
FROM canonical_chains
ON CONFLICT (slug) DO UPDATE
SET symbol = EXCLUDED.symbol,
    name = CASE
      WHEN chains.name = '' OR chains.name = chains.symbol THEN EXCLUDED.name
      ELSE chains.name
    END,
    evm_chain_id = COALESCE(chains.evm_chain_id, EXCLUDED.evm_chain_id);

WITH chain_merge_map(source_slug, target_slug) AS (
  VALUES
    ('baseevm', 'base'),
    ('arbevm', 'arbitrum'),
    ('arbitrumone', 'arbitrum'),
    ('polygon-pos', 'polygon'),
    ('opeth', 'optimism'),
    ('lineaeth', 'linea'),
    ('avax-c', 'avax'),
    ('avaxc-chain', 'avax'),
    ('avax-c-chain', 'avax'),
    ('avalanche-c-chain', 'avax'),
    ('blasteth', 'blast'),
    ('zk-eth', 'zksyncera'),
    ('zksera', 'zksyncera')
), pairs AS (
  SELECT source.id AS source_id, target.id AS target_id
  FROM chain_merge_map m
  JOIN chains source ON source.slug = m.source_slug
  JOIN chains target ON target.slug = m.target_slug
)
UPDATE chains child
SET parent_chain_id = pairs.target_id
FROM pairs
WHERE child.parent_chain_id = pairs.source_id;

WITH chain_merge_map(source_slug, target_slug) AS (
  VALUES
    ('baseevm', 'base'),
    ('arbevm', 'arbitrum'),
    ('arbitrumone', 'arbitrum'),
    ('polygon-pos', 'polygon'),
    ('opeth', 'optimism'),
    ('lineaeth', 'linea'),
    ('avax-c', 'avax'),
    ('avaxc-chain', 'avax'),
    ('avax-c-chain', 'avax'),
    ('avalanche-c-chain', 'avax'),
    ('blasteth', 'blast'),
    ('zk-eth', 'zksyncera'),
    ('zksera', 'zksyncera')
), pairs AS (
  SELECT source.id AS source_id, target.id AS target_id
  FROM chain_merge_map m
  JOIN chains source ON source.slug = m.source_slug
  JOIN chains target ON target.slug = m.target_slug
)
UPDATE chain_aliases ca
SET chain_id = pairs.target_id
FROM pairs
WHERE ca.chain_id = pairs.source_id;

WITH chain_merge_map(source_slug, target_slug) AS (
  VALUES
    ('baseevm', 'base'),
    ('arbevm', 'arbitrum'),
    ('arbitrumone', 'arbitrum'),
    ('polygon-pos', 'polygon'),
    ('opeth', 'optimism'),
    ('lineaeth', 'linea'),
    ('avax-c', 'avax'),
    ('avaxc-chain', 'avax'),
    ('avax-c-chain', 'avax'),
    ('avalanche-c-chain', 'avax'),
    ('blasteth', 'blast'),
    ('zk-eth', 'zksyncera'),
    ('zksera', 'zksyncera')
), pairs AS (
  SELECT source.id AS source_id, target.id AS target_id
  FROM chain_merge_map m
  JOIN chains source ON source.slug = m.source_slug
  JOIN chains target ON target.slug = m.target_slug
)
UPDATE alert_rules ar
SET chain_id = pairs.target_id
FROM pairs
WHERE ar.chain_id = pairs.source_id;

WITH chain_merge_map(source_slug, target_slug) AS (
  VALUES
    ('baseevm', 'base'),
    ('arbevm', 'arbitrum'),
    ('arbitrumone', 'arbitrum'),
    ('polygon-pos', 'polygon'),
    ('opeth', 'optimism'),
    ('lineaeth', 'linea'),
    ('avax-c', 'avax'),
    ('avaxc-chain', 'avax'),
    ('avax-c-chain', 'avax'),
    ('avalanche-c-chain', 'avax'),
    ('blasteth', 'blast'),
    ('zk-eth', 'zksyncera'),
    ('zksera', 'zksyncera')
), pairs AS (
  SELECT source.id AS source_id, target.id AS target_id
  FROM chain_merge_map m
  JOIN chains source ON source.slug = m.source_slug
  JOIN chains target ON target.slug = m.target_slug
)
UPDATE rail_events re
SET chain_id = pairs.target_id
FROM pairs
WHERE re.chain_id = pairs.source_id;

WITH chain_merge_map(source_slug, target_slug) AS (
  VALUES
    ('baseevm', 'base'),
    ('arbevm', 'arbitrum'),
    ('arbitrumone', 'arbitrum'),
    ('polygon-pos', 'polygon'),
    ('opeth', 'optimism'),
    ('lineaeth', 'linea'),
    ('avax-c', 'avax'),
    ('avaxc-chain', 'avax'),
    ('avax-c-chain', 'avax'),
    ('avalanche-c-chain', 'avax'),
    ('blasteth', 'blast'),
    ('zk-eth', 'zksyncera'),
    ('zksera', 'zksyncera')
), pairs AS (
  SELECT source.id AS source_id, target.id AS target_id
  FROM chain_merge_map m
  JOIN chains source ON source.slug = m.source_slug
  JOIN chains target ON target.slug = m.target_slug
), conflicts AS (
  SELECT source_rail.id AS source_rail_id,
         target_rail.id AS target_rail_id
  FROM pairs
  JOIN rails source_rail ON source_rail.chain_id = pairs.source_id
  JOIN rails target_rail
    ON target_rail.exchange_id = source_rail.exchange_id
   AND target_rail.coin_id = source_rail.coin_id
   AND target_rail.chain_id = pairs.target_id
)
UPDATE rails target_rail
SET deposit_enabled = source_rail.deposit_enabled,
    withdraw_enabled = source_rail.withdraw_enabled,
    deposit_confirmations = COALESCE(source_rail.deposit_confirmations, target_rail.deposit_confirmations),
    withdraw_min = COALESCE(source_rail.withdraw_min, target_rail.withdraw_min),
    withdraw_fee = COALESCE(source_rail.withdraw_fee, target_rail.withdraw_fee),
    withdraw_fee_type = COALESCE(source_rail.withdraw_fee_type, target_rail.withdraw_fee_type),
    withdraw_fee_percent = COALESCE(source_rail.withdraw_fee_percent, target_rail.withdraw_fee_percent),
    deposit_off_started_at = source_rail.deposit_off_started_at,
    withdraw_off_started_at = source_rail.withdraw_off_started_at,
    is_active = source_rail.is_active,
    missing_since = source_rail.missing_since,
    missing_count = source_rail.missing_count,
    is_initial = source_rail.is_initial,
    last_seen_at = GREATEST(source_rail.last_seen_at, target_rail.last_seen_at)
FROM conflicts
JOIN rails source_rail ON source_rail.id = conflicts.source_rail_id
WHERE target_rail.id = conflicts.target_rail_id;

WITH chain_merge_map(source_slug, target_slug) AS (
  VALUES
    ('baseevm', 'base'),
    ('arbevm', 'arbitrum'),
    ('arbitrumone', 'arbitrum'),
    ('polygon-pos', 'polygon'),
    ('opeth', 'optimism'),
    ('lineaeth', 'linea'),
    ('avax-c', 'avax'),
    ('avaxc-chain', 'avax'),
    ('avax-c-chain', 'avax'),
    ('avalanche-c-chain', 'avax'),
    ('blasteth', 'blast'),
    ('zk-eth', 'zksyncera'),
    ('zksera', 'zksyncera')
), pairs AS (
  SELECT source.id AS source_id, target.id AS target_id
  FROM chain_merge_map m
  JOIN chains source ON source.slug = m.source_slug
  JOIN chains target ON target.slug = m.target_slug
), conflicts AS (
  SELECT source_rail.id AS source_rail_id
  FROM pairs
  JOIN rails source_rail ON source_rail.chain_id = pairs.source_id
  JOIN rails target_rail
    ON target_rail.exchange_id = source_rail.exchange_id
   AND target_rail.coin_id = source_rail.coin_id
   AND target_rail.chain_id = pairs.target_id
)
DELETE FROM rails r
USING conflicts
WHERE r.id = conflicts.source_rail_id;

WITH chain_merge_map(source_slug, target_slug) AS (
  VALUES
    ('baseevm', 'base'),
    ('arbevm', 'arbitrum'),
    ('arbitrumone', 'arbitrum'),
    ('polygon-pos', 'polygon'),
    ('opeth', 'optimism'),
    ('lineaeth', 'linea'),
    ('avax-c', 'avax'),
    ('avaxc-chain', 'avax'),
    ('avax-c-chain', 'avax'),
    ('avalanche-c-chain', 'avax'),
    ('blasteth', 'blast'),
    ('zk-eth', 'zksyncera'),
    ('zksera', 'zksyncera')
), pairs AS (
  SELECT source.id AS source_id, target.id AS target_id
  FROM chain_merge_map m
  JOIN chains source ON source.slug = m.source_slug
  JOIN chains target ON target.slug = m.target_slug
)
UPDATE rails r
SET chain_id = pairs.target_id
FROM pairs
WHERE r.chain_id = pairs.source_id;

WITH chain_merge_map(source_slug, target_slug) AS (
  VALUES
    ('baseevm', 'base'),
    ('arbevm', 'arbitrum'),
    ('arbitrumone', 'arbitrum'),
    ('polygon-pos', 'polygon'),
    ('opeth', 'optimism'),
    ('lineaeth', 'linea'),
    ('avax-c', 'avax'),
    ('avaxc-chain', 'avax'),
    ('avax-c-chain', 'avax'),
    ('avalanche-c-chain', 'avax'),
    ('blasteth', 'blast'),
    ('zk-eth', 'zksyncera'),
    ('zksera', 'zksyncera')
), source_chains AS (
  SELECT source.id
  FROM chain_merge_map m
  JOIN chains source ON source.slug = m.source_slug
)
DELETE FROM chains ch
USING source_chains
WHERE ch.id = source_chains.id
  AND NOT EXISTS (SELECT 1 FROM rails r WHERE r.chain_id = ch.id)
  AND NOT EXISTS (SELECT 1 FROM chain_aliases ca WHERE ca.chain_id = ch.id)
  AND NOT EXISTS (SELECT 1 FROM alert_rules ar WHERE ar.chain_id = ch.id)
  AND NOT EXISTS (SELECT 1 FROM chains child WHERE child.parent_chain_id = ch.id);

-- +goose Down
-- This migration folds duplicate chain rows into canonical chains. Recreating the
-- duplicate rows would lose canonical rail history, so the down migration is
-- intentionally left as a no-op.
