-- +goose Up
UPDATE coins
SET external_ids = external_ids || '{"coingecko":"tether"}'::jsonb
WHERE slug = 'usdt';

UPDATE coins
SET external_ids = external_ids || '{"coingecko":"usd-coin"}'::jsonb
WHERE slug = 'usdc';

UPDATE coins
SET external_ids = external_ids || '{"coingecko":"worldcoin-wld"}'::jsonb
WHERE slug = 'wld';

UPDATE coins
SET external_ids = external_ids || '{"coingecko":"layerzero"}'::jsonb
WHERE slug = 'zro';

UPDATE coins
SET external_ids = external_ids || '{"coingecko":"starknet"}'::jsonb
WHERE slug = 'strk';

UPDATE coins
SET external_ids = external_ids || '{"coingecko":"sign"}'::jsonb
WHERE slug = 'sign';

-- +goose Down
UPDATE coins
SET external_ids = external_ids - 'coingecko'
WHERE slug IN ('usdt', 'usdc', 'wld', 'zro', 'strk', 'sign');
