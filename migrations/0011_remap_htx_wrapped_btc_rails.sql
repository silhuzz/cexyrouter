-- +goose Up
WITH remap(chain_slug, target_coin_slug) AS (
  VALUES
    ('ethereum', 'wbtc'),
    ('base', 'cbbtc')
), ids AS (
  SELECT e.id AS exchange_id,
         source_coin.id AS source_coin_id,
         target_coin.id AS target_coin_id,
         ch.id AS chain_id
  FROM remap
  JOIN exchanges e ON e.slug = 'htx'
  JOIN coins source_coin ON source_coin.slug = 'btc'
  JOIN coins target_coin ON target_coin.slug = remap.target_coin_slug
  JOIN chains ch ON ch.slug = remap.chain_slug
), conflicts AS (
  SELECT source_rail.id AS source_rail_id,
         target_rail.id AS target_rail_id
  FROM ids
  JOIN rails source_rail
    ON source_rail.exchange_id = ids.exchange_id
   AND source_rail.coin_id = ids.source_coin_id
   AND source_rail.chain_id = ids.chain_id
  JOIN rails target_rail
    ON target_rail.exchange_id = source_rail.exchange_id
   AND target_rail.coin_id = ids.target_coin_id
   AND target_rail.chain_id = source_rail.chain_id
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

WITH remap(chain_slug, target_coin_slug) AS (
  VALUES
    ('ethereum', 'wbtc'),
    ('base', 'cbbtc')
), ids AS (
  SELECT e.id AS exchange_id,
         source_coin.id AS source_coin_id,
         target_coin.id AS target_coin_id,
         ch.id AS chain_id
  FROM remap
  JOIN exchanges e ON e.slug = 'htx'
  JOIN coins source_coin ON source_coin.slug = 'btc'
  JOIN coins target_coin ON target_coin.slug = remap.target_coin_slug
  JOIN chains ch ON ch.slug = remap.chain_slug
), conflicts AS (
  SELECT source_rail.id AS source_rail_id
  FROM ids
  JOIN rails source_rail
    ON source_rail.exchange_id = ids.exchange_id
   AND source_rail.coin_id = ids.source_coin_id
   AND source_rail.chain_id = ids.chain_id
  JOIN rails target_rail
    ON target_rail.exchange_id = source_rail.exchange_id
   AND target_rail.coin_id = ids.target_coin_id
   AND target_rail.chain_id = source_rail.chain_id
)
DELETE FROM rails r
USING conflicts
WHERE r.id = conflicts.source_rail_id;

WITH remap(chain_slug, target_coin_slug) AS (
  VALUES
    ('ethereum', 'wbtc'),
    ('base', 'cbbtc')
), ids AS (
  SELECT e.id AS exchange_id,
         source_coin.id AS source_coin_id,
         target_coin.id AS target_coin_id,
         ch.id AS chain_id
  FROM remap
  JOIN exchanges e ON e.slug = 'htx'
  JOIN coins source_coin ON source_coin.slug = 'btc'
  JOIN coins target_coin ON target_coin.slug = remap.target_coin_slug
  JOIN chains ch ON ch.slug = remap.chain_slug
)
UPDATE rails r
SET coin_id = ids.target_coin_id
FROM ids
WHERE r.exchange_id = ids.exchange_id
  AND r.coin_id = ids.source_coin_id
  AND r.chain_id = ids.chain_id;

-- +goose Down
SELECT 1;
