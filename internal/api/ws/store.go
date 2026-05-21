package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/silhuzz/cexyrouter/internal/api/eventmeta"
)

type EventLookup interface {
	EventByID(ctx context.Context, id int64) (Event, error)
}

type BackfillStore interface {
	Backfill(ctx context.Context, after Cursor, filters Filters, limit int) ([]Event, error)
}

type EventStore interface {
	EventLookup
	BackfillStore
}

type SQLStore struct {
	db *pgxpool.Pool
}

func NewSQLStore(db *pgxpool.Pool) *SQLStore {
	return &SQLStore{db: db}
}

func (s *SQLStore) EventByID(ctx context.Context, id int64) (Event, error) {
	if s == nil || s.db == nil {
		return Event{}, fmt.Errorf("database pool is not configured")
	}
	row := s.db.QueryRow(ctx, eventSelectSQL("re.id = $1", "ORDER BY re.occurred_at DESC, re.id DESC LIMIT 1"), id)
	event, err := scanEvent(row)
	if err != nil {
		return Event{}, err
	}
	return attachCursor(event)
}

func (s *SQLStore) Backfill(ctx context.Context, after Cursor, filters Filters, limit int) ([]Event, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("database pool is not configured")
	}
	if limit <= 0 || limit > backfillBatchLimit {
		limit = backfillBatchLimit
	}
	var err error
	filters, err = NormalizeFilters(filters)
	if err != nil {
		return nil, err
	}
	if filters.matchNothing() {
		return []Event{}, nil
	}

	clauses := []string{"(re.occurred_at, re.id) > ($1, $2)"}
	args := []any{after.OccurredAt, after.ID}
	addArrayFilter := func(sql string, values []string) {
		if values == nil {
			return
		}
		args = append(args, values)
		clauses = append(clauses, fmt.Sprintf(sql, len(args)))
	}

	addArrayFilter("e.slug = ANY($%d::text[])", filters.Exchange)
	addArrayFilter("c.slug = ANY($%d::text[])", filters.Coin)
	addArrayFilter("ch.slug = ANY($%d::text[])", filters.Chain)
	addArrayFilter("re.event_type = ANY($%d::text[])", filters.EventType)

	args = append(args, limit)
	rows, err := s.db.Query(ctx, eventSelectSQL(strings.Join(clauses, " AND "), fmt.Sprintf("ORDER BY re.occurred_at ASC, re.id ASC LIMIT $%d", len(args))), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]Event, 0, limit)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		event, err = attachCursor(event)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func BackfillSince(ctx context.Context, store BackfillStore, since Cursor, filters Filters, send func(Event) error) (Cursor, error) {
	if store == nil {
		return since, fmt.Errorf("backfill store is required")
	}
	if send == nil {
		return since, fmt.Errorf("backfill send callback is required")
	}
	cursor := since
	for {
		events, err := store.Backfill(ctx, cursor, filters, backfillBatchLimit)
		if err != nil {
			return cursor, err
		}
		for _, event := range events {
			if err := send(event); err != nil {
				return cursor, err
			}
			cursor = cursorFromEvent(event)
		}
		if len(events) < backfillBatchLimit {
			return cursor, nil
		}
	}
}

func eventSelectSQL(where string, suffix string) string {
	return fmt.Sprintf(`
		SELECT
			re.id,
			re.rail_id,
			re.event_type,
			e.id, e.slug, e.name, e.region,
			c.id, c.slug, c.symbol, c.name,
			ch.id, ch.slug, ch.symbol, ch.name, ch.evm_chain_id, ch.parent_chain_id,
			re.before,
			re.after,
			re.occurred_at
		FROM rail_events re
		JOIN exchanges e ON e.id = re.exchange_id
		JOIN coins c ON c.id = re.coin_id
		JOIN chains ch ON ch.id = re.chain_id
		WHERE %s
		%s
	`, where, suffix)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(scanner rowScanner) (Event, error) {
	var event Event
	var evmChainID pgtype.Int4
	var parentChainID pgtype.Int4
	var before []byte
	var after []byte
	if err := scanner.Scan(
		&event.ID,
		&event.RailID,
		&event.EventType,
		&event.Exchange.ID,
		&event.Exchange.Slug,
		&event.Exchange.Name,
		&event.Exchange.Region,
		&event.Coin.ID,
		&event.Coin.Slug,
		&event.Coin.Symbol,
		&event.Coin.Name,
		&event.Chain.ID,
		&event.Chain.Slug,
		&event.Chain.Symbol,
		&event.Chain.Name,
		&evmChainID,
		&parentChainID,
		&before,
		&after,
		&event.OccurredAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return Event{}, err
		}
		return Event{}, err
	}
	event.Chain.EVMChainID = int32Ptr(evmChainID)
	event.Chain.ParentChainID = int32Ptr(parentChainID)
	event.Before = json.RawMessage(defaultJSONObject(before))
	event.After = json.RawMessage(defaultJSONObject(after))
	details := eventmeta.Build(event.EventType, event.Before, event.After)
	event.Summary = details.Summary
	event.Changes = details.Changes
	return event, nil
}

func defaultJSONObject(value []byte) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	return value
}

func int32Ptr(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}
