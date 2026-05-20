-- +goose Up
UPDATE coins
SET external_ids = jsonb_set(external_ids, '{coingecko}', '"sign-global"', true)
WHERE slug = 'sign';

-- +goose Down
UPDATE coins
SET external_ids = jsonb_set(external_ids, '{coingecko}', '"sign"', true)
WHERE slug = 'sign';
