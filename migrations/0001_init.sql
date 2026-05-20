-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
    CREATE EXTENSION IF NOT EXISTS timescaledb;
  ELSE
    RAISE NOTICE 'timescaledb extension is unavailable; rail_events will run as a regular Postgres table for local smoke tests';
  END IF;
END;
$$;
-- +goose StatementEnd

CREATE TABLE exchanges (
  id            SERIAL PRIMARY KEY,
  slug          TEXT UNIQUE NOT NULL,
  name          TEXT NOT NULL,
  region        TEXT NOT NULL
);

INSERT INTO exchanges (slug, name, region) VALUES
  ('binance', 'Binance', 'Global'),
  ('bybit', 'Bybit', 'Global'),
  ('okx', 'OKX', 'Global'),
  ('bithumb', 'Bithumb', 'Korea'),
  ('upbit', 'Upbit', 'Korea')
ON CONFLICT (slug) DO NOTHING;

CREATE TABLE coins (
  id            SERIAL PRIMARY KEY,
  slug          TEXT UNIQUE NOT NULL,
  symbol        TEXT NOT NULL,
  name          TEXT NOT NULL,
  external_ids  JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE coin_aliases (
  exchange_id        INT  NOT NULL REFERENCES exchanges(id),
  raw_symbol         TEXT NOT NULL,
  raw_name           TEXT NOT NULL DEFAULT '',
  coin_id            INT  NOT NULL REFERENCES coins(id),
  confidence         SMALLINT NOT NULL DEFAULT 1,
  first_seen         TIMESTAMPTZ NOT NULL,
  last_seen          TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (exchange_id, raw_symbol, raw_name)
);

CREATE TABLE chains (
  id              SERIAL PRIMARY KEY,
  slug            TEXT UNIQUE NOT NULL,
  symbol          TEXT NOT NULL,
  name            TEXT NOT NULL,
  evm_chain_id    INT  NULL,
  parent_chain_id INT  REFERENCES chains(id) NULL
);

CREATE TABLE chain_aliases (
  exchange_id        INT  NOT NULL REFERENCES exchanges(id),
  raw_symbol         TEXT NOT NULL,
  raw_name           TEXT NOT NULL DEFAULT '',
  raw_network_id     TEXT NOT NULL DEFAULT '',
  chain_id           INT  NOT NULL REFERENCES chains(id),
  confidence         SMALLINT NOT NULL DEFAULT 1,
  first_seen         TIMESTAMPTZ NOT NULL,
  last_seen          TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (exchange_id, raw_symbol, raw_name, raw_network_id)
);

CREATE TABLE rails (
  id                       BIGSERIAL PRIMARY KEY,
  exchange_id              INT  NOT NULL REFERENCES exchanges(id),
  coin_id                  INT  NOT NULL REFERENCES coins(id),
  chain_id                 INT  NOT NULL REFERENCES chains(id),
  deposit_enabled          BOOLEAN NOT NULL,
  withdraw_enabled         BOOLEAN NOT NULL,
  deposit_confirmations    INT     NULL,
  withdraw_min             NUMERIC NULL,
  withdraw_fee             NUMERIC NULL,
  withdraw_fee_type        TEXT    NULL CHECK (withdraw_fee_type IN ('fixed','percent','hybrid')),
  withdraw_fee_percent     NUMERIC NULL,
  deposit_off_started_at   TIMESTAMPTZ NULL,
  withdraw_off_started_at  TIMESTAMPTZ NULL,
  is_active                BOOLEAN NOT NULL DEFAULT TRUE,
  missing_since            TIMESTAMPTZ NULL,
  missing_count            INT NOT NULL DEFAULT 0,
  is_initial               BOOLEAN NOT NULL DEFAULT TRUE,
  last_seen_at             TIMESTAMPTZ NOT NULL,
  UNIQUE (exchange_id, coin_id, chain_id)
);

CREATE INDEX rails_lookup ON rails (coin_id, chain_id, exchange_id);
CREATE INDEX rails_active ON rails (is_active) WHERE is_active = TRUE;

CREATE TABLE rail_events (
  id            BIGSERIAL NOT NULL,
  rail_id       BIGINT    NOT NULL,
  exchange_id   INT       NOT NULL,
  coin_id       INT       NOT NULL,
  chain_id      INT       NOT NULL,
  event_type    TEXT      NOT NULL CHECK (event_type IN (
                  'deposit_off','deposit_on','withdraw_off','withdraw_on',
                  'fee_changed','min_changed','rail_delisted','rail_relisted'
                )),
  before        JSONB     NOT NULL,
  after         JSONB     NOT NULL,
  occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (occurred_at, id)
);

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
    PERFORM create_hypertable('rail_events', 'occurred_at', if_not_exists => TRUE);
    PERFORM add_retention_policy('rail_events', INTERVAL '90 days', if_not_exists => TRUE);
  END IF;
END;
$$;
-- +goose StatementEnd

CREATE INDEX rail_events_id ON rail_events (id);
CREATE INDEX rail_events_recent ON rail_events (occurred_at DESC, id DESC);
CREATE INDEX rail_events_filter ON rail_events (exchange_id, occurred_at DESC);
CREATE INDEX rail_events_by_type ON rail_events (event_type, occurred_at DESC);

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

CREATE TRIGGER rail_events_notify AFTER INSERT ON rail_events
FOR EACH ROW EXECUTE FUNCTION notify_rail_event();

CREATE TABLE adapter_freshness (
  exchange_id            INT PRIMARY KEY REFERENCES exchanges(id),
  last_successful_poll   TIMESTAMPTZ NULL,
  last_attempt           TIMESTAMPTZ NULL,
  last_error             TEXT NULL,
  consecutive_failures   INT  NOT NULL DEFAULT 0
);

INSERT INTO adapter_freshness (exchange_id)
SELECT id FROM exchanges
ON CONFLICT (exchange_id) DO NOTHING;

CREATE TABLE tg_users (
  id          SERIAL PRIMARY KEY,
  tg_chat_id  BIGINT UNIQUE NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alert_rules (
  id            SERIAL PRIMARY KEY,
  tg_user_id    INT NOT NULL REFERENCES tg_users(id) ON DELETE CASCADE,
  exchange_id   INT NULL REFERENCES exchanges(id),
  coin_id       INT NULL REFERENCES coins(id),
  chain_id      INT NULL REFERENCES chains(id),
  event_types   TEXT[] NOT NULL CHECK (array_length(event_types, 1) > 0),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX alert_rules_filter ON alert_rules USING GIN (event_types);
CREATE INDEX alert_rules_by_user ON alert_rules (tg_user_id);

CREATE TABLE notification_jobs (
  id              BIGSERIAL PRIMARY KEY,
  event_id        BIGINT NOT NULL,
  event_occurred_at TIMESTAMPTZ NOT NULL,
  tg_chat_id      BIGINT NOT NULL,
  body            TEXT   NOT NULL,
  status          TEXT   NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','in_flight','sent','failed','dead')),
  attempts        INT    NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error      TEXT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at         TIMESTAMPTZ NULL,
  UNIQUE (event_id, tg_chat_id)
);

CREATE INDEX notification_jobs_due ON notification_jobs (next_attempt_at)
  WHERE status IN ('pending','in_flight');

-- +goose Down
DROP INDEX IF EXISTS notification_jobs_due;
DROP TABLE IF EXISTS notification_jobs;
DROP INDEX IF EXISTS alert_rules_by_user;
DROP INDEX IF EXISTS alert_rules_filter;
DROP TABLE IF EXISTS alert_rules;
DROP TABLE IF EXISTS tg_users;
DROP TABLE IF EXISTS adapter_freshness;
DROP TRIGGER IF EXISTS rail_events_notify ON rail_events;
DROP FUNCTION IF EXISTS notify_rail_event();
DROP INDEX IF EXISTS rail_events_by_type;
DROP INDEX IF EXISTS rail_events_filter;
DROP INDEX IF EXISTS rail_events_recent;
DROP INDEX IF EXISTS rail_events_id;
DROP TABLE IF EXISTS rail_events;
DROP INDEX IF EXISTS rails_active;
DROP INDEX IF EXISTS rails_lookup;
DROP TABLE IF EXISTS rails;
DROP TABLE IF EXISTS chain_aliases;
DROP TABLE IF EXISTS chains;
DROP TABLE IF EXISTS coin_aliases;
DROP TABLE IF EXISTS coins;
DROP TABLE IF EXISTS exchanges;
DROP EXTENSION IF EXISTS timescaledb;
