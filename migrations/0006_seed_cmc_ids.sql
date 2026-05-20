-- +goose Up
UPDATE coins
SET external_ids = external_ids || '{"cmc_id":1,"cmc_slug":"bitcoin"}'::jsonb
WHERE slug = 'btc';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":3717,"cmc_slug":"wrapped-bitcoin"}'::jsonb
WHERE slug = 'wbtc';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":26133,"cmc_slug":"tbtc-token"}'::jsonb
WHERE slug = 'tbtc';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":32994,"cmc_slug":"coinbase-wrapped-btc"}'::jsonb
WHERE slug = 'cbbtc';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":1027,"cmc_slug":"ethereum"}'::jsonb
WHERE slug = 'eth';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":2396,"cmc_slug":"weth"}'::jsonb
WHERE slug = 'weth';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":825,"cmc_slug":"tether"}'::jsonb
WHERE slug = 'usdt';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":3408,"cmc_slug":"usd-coin"}'::jsonb
WHERE slug = 'usdc';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":13502,"cmc_slug":"worldcoin-org"}'::jsonb
WHERE slug = 'wld';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":26997,"cmc_slug":"layerzero"}'::jsonb
WHERE slug = 'zro';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":22691,"cmc_slug":"starknet-token"}'::jsonb
WHERE slug = 'strk';

UPDATE coins
SET external_ids = external_ids || '{"cmc_id":35600,"cmc_slug":"sign"}'::jsonb
WHERE slug = 'sign';

-- +goose Down
UPDATE coins
SET external_ids = external_ids - 'cmc_id' - 'cmc_slug'
WHERE slug IN ('btc', 'wbtc', 'tbtc', 'cbbtc', 'eth', 'weth', 'usdt', 'usdc', 'wld', 'zro', 'strk', 'sign');
