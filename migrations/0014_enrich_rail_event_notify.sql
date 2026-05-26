-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_rail_event() RETURNS TRIGGER AS $$
DECLARE
  payload TEXT;
BEGIN
  SELECT json_build_object(
    'id', NEW.id,
    'rail_id', NEW.rail_id,
    'occurred_at', NEW.occurred_at,
    'event_type', NEW.event_type,
    'exchange_id', NEW.exchange_id,
    'coin_id', NEW.coin_id,
    'chain_id', NEW.chain_id,
    'exchange', json_build_object(
      'id', e.id,
      'slug', e.slug,
      'name', e.name,
      'region', e.region
    ),
    'coin', json_build_object(
      'id', c.id,
      'slug', c.slug,
      'symbol', c.symbol,
      'name', c.name
    ),
    'chain', json_build_object(
      'id', ch.id,
      'slug', ch.slug,
      'symbol', ch.symbol,
      'name', ch.name,
      'evm_chain_id', ch.evm_chain_id,
      'parent_chain_id', ch.parent_chain_id
    ),
    'before', NEW.before,
    'after', NEW.after
  )::text
  INTO payload
  FROM (SELECT 1) AS anchor
  LEFT JOIN exchanges e ON e.id = NEW.exchange_id
  LEFT JOIN coins c ON c.id = NEW.coin_id
  LEFT JOIN chains ch ON ch.id = NEW.chain_id;

  PERFORM pg_notify('rail_events', payload);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_rail_event() RETURNS TRIGGER AS $$
BEGIN
  PERFORM pg_notify('rail_events', json_build_object(
    'id', NEW.id,
    'occurred_at', NEW.occurred_at,
    'event_type', NEW.event_type,
    'exchange_id', NEW.exchange_id,
    'coin_id', NEW.coin_id,
    'chain_id', NEW.chain_id
  )::text);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
