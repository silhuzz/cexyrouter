package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type SQLRepository struct {
	db DBTX
}

func NewSQLRepository(db DBTX) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) EnsureUser(ctx context.Context, chatID int64) (User, error) {
	var user User
	err := r.db.QueryRow(ctx, `
		INSERT INTO tg_users (tg_chat_id)
		VALUES ($1)
		ON CONFLICT (tg_chat_id) DO UPDATE
		SET tg_chat_id = EXCLUDED.tg_chat_id
		RETURNING id, tg_chat_id
	`, chatID).Scan(&user.ID, &user.TGChatID)
	if err != nil {
		return User{}, fmt.Errorf("ensure telegram user: %w", err)
	}
	return user, nil
}

func (r *SQLRepository) CreateRule(ctx context.Context, chatID int64, cmd SubscribeCommand) (Rule, error) {
	user, err := r.EnsureUser(ctx, chatID)
	if err != nil {
		return Rule{}, err
	}

	exchange, err := r.resolveReference(ctx, "exchange", cmd.Exchange)
	if err != nil {
		return Rule{}, err
	}
	coin, err := r.resolveReference(ctx, "coin", cmd.Coin)
	if err != nil {
		return Rule{}, err
	}
	chain, err := r.resolveReference(ctx, "chain", cmd.Chain)
	if err != nil {
		return Rule{}, err
	}

	rule := Rule{
		Exchange:   referenceSlug(exchange),
		Coin:       referenceSlug(coin),
		Chain:      referenceSlug(chain),
		EventTypes: append([]string(nil), cmd.EventTypes...),
	}
	err = r.db.QueryRow(ctx, `
		INSERT INTO alert_rules (tg_user_id, exchange_id, coin_id, chain_id, event_types)
		VALUES ($1, $2, $3, $4, $5::text[])
		RETURNING id, created_at
	`,
		user.ID,
		referenceIDArg(exchange),
		referenceIDArg(coin),
		referenceIDArg(chain),
		cmd.EventTypes,
	).Scan(&rule.ID, &rule.CreatedAt)
	if err != nil {
		return Rule{}, fmt.Errorf("create alert rule: %w", err)
	}
	return rule, nil
}

func (r *SQLRepository) ListRules(ctx context.Context, chatID int64) ([]Rule, error) {
	if _, err := r.EnsureUser(ctx, chatID); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT ar.id,
		       COALESCE(e.slug, ''),
		       COALESCE(c.slug, ''),
		       COALESCE(ch.slug, ''),
		       ar.event_types,
		       ar.created_at
		FROM tg_users u
		JOIN alert_rules ar ON ar.tg_user_id = u.id
		LEFT JOIN exchanges e ON e.id = ar.exchange_id
		LEFT JOIN coins c ON c.id = ar.coin_id
		LEFT JOIN chains ch ON ch.id = ar.chain_id
		WHERE u.tg_chat_id = $1
		  AND (
		      ar.exchange_id IS NULL
		      OR EXISTS (
		          SELECT 1
		          FROM adapter_freshness af
		          WHERE af.exchange_id = ar.exchange_id
		            AND (
		                af.last_successful_poll IS NOT NULL
		                OR af.last_attempt IS NOT NULL
		                OR af.last_error IS NOT NULL
		            )
		      )
		  )
		ORDER BY ar.id
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer rows.Close()

	rules := make([]Rule, 0)
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(&rule.ID, &rule.Exchange, &rule.Coin, &rule.Chain, &rule.EventTypes, &rule.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan alert rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert rules: %w", err)
	}
	return rules, nil
}

func (r *SQLRepository) DeleteRule(ctx context.Context, chatID int64, ruleID int64) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM alert_rules ar
		USING tg_users u
		WHERE ar.tg_user_id = u.id
		  AND u.tg_chat_id = $1
		  AND ar.id = $2
	`, chatID, ruleID)
	if err != nil {
		return false, fmt.Errorf("delete alert rule: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *SQLRepository) DeleteRules(ctx context.Context, chatID int64) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM alert_rules ar
		USING tg_users u
		WHERE ar.tg_user_id = u.id
		  AND u.tg_chat_id = $1
	`, chatID)
	if err != nil {
		return 0, fmt.Errorf("delete alert rules: %w", err)
	}

	if _, err := r.db.Exec(ctx, `
		DELETE FROM notification_jobs
		WHERE tg_chat_id = $1
		  AND status IN ('pending','in_flight')
	`, chatID); err != nil {
		return tag.RowsAffected(), fmt.Errorf("delete queued notification jobs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *SQLRepository) FindRails(ctx context.Context, filter StatusCommand, limit int) ([]RailStatus, error) {
	if limit <= 0 {
		limit = defaultStatusLimit
	}

	clauses := []string{"1=1"}
	args := make([]any, 0, 4)
	addFilter := func(sql string, value string, placeholderCount int) {
		args = append(args, value)
		idx := len(args)
		switch placeholderCount {
		case 2:
			clauses = append(clauses, fmt.Sprintf(sql, idx, idx))
		default:
			clauses = append(clauses, fmt.Sprintf(sql, idx, idx, idx))
		}
	}

	if filter.Exchange != "" {
		addFilter("(LOWER(e.slug) = LOWER($%d) OR LOWER(e.name) = LOWER($%d))", filter.Exchange, 2)
	}
	if filter.Coin != "" {
		addFilter("(LOWER(c.slug) = LOWER($%d) OR LOWER(c.symbol) = LOWER($%d) OR LOWER(c.name) = LOWER($%d))", filter.Coin, 3)
	}
	if filter.Chain != "" {
		addFilter("(LOWER(ch.slug) = LOWER($%d) OR LOWER(ch.symbol) = LOWER($%d) OR LOWER(ch.name) = LOWER($%d))", filter.Chain, 3)
	}

	args = append(args, limit)
	sql := fmt.Sprintf(`
		SELECT e.slug,
		       c.symbol,
		       ch.slug,
		       r.deposit_enabled,
		       r.withdraw_enabled,
		       r.is_active,
		       r.is_initial,
		       COALESCE(r.withdraw_min::text, ''),
		       COALESCE(r.withdraw_fee::text, ''),
		       r.last_seen_at
		FROM rails r
		JOIN exchanges e ON e.id = r.exchange_id
		JOIN adapter_freshness af ON af.exchange_id = e.id
		JOIN coins c ON c.id = r.coin_id
		JOIN chains ch ON ch.id = r.chain_id
		WHERE %s
		  AND (
		      af.last_successful_poll IS NOT NULL
		      OR af.last_attempt IS NOT NULL
		      OR af.last_error IS NOT NULL
		  )
		ORDER BY c.slug, ch.slug, e.slug
		LIMIT $%d
	`, strings.Join(clauses, " AND "), len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("find rail status: %w", err)
	}
	defer rows.Close()

	rails := make([]RailStatus, 0)
	for rows.Next() {
		var rail RailStatus
		if err := rows.Scan(
			&rail.Exchange,
			&rail.Coin,
			&rail.Chain,
			&rail.DepositEnabled,
			&rail.WithdrawEnabled,
			&rail.IsActive,
			&rail.IsInitial,
			&rail.WithdrawMin,
			&rail.WithdrawFee,
			&rail.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("scan rail status: %w", err)
		}
		rails = append(rails, rail)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rail status: %w", err)
	}
	return rails, nil
}

type reference struct {
	id   int32
	slug string
}

func (r *SQLRepository) resolveReference(ctx context.Context, kind string, value string) (*reference, error) {
	if value == "" {
		return nil, nil
	}

	var sql string
	switch kind {
	case "exchange":
		sql = `
			SELECT e.id, e.slug
			FROM exchanges e
			JOIN adapter_freshness af ON af.exchange_id = e.id
			WHERE (LOWER(e.slug) = LOWER($1) OR LOWER(e.name) = LOWER($1))
			  AND (
			      af.last_successful_poll IS NOT NULL
			      OR af.last_attempt IS NOT NULL
			      OR af.last_error IS NOT NULL
			  )
			LIMIT 1
		`
	case "coin":
		sql = `
			SELECT id, slug
			FROM coins
			WHERE LOWER(slug) = LOWER($1) OR LOWER(symbol) = LOWER($1) OR LOWER(name) = LOWER($1)
			LIMIT 1
		`
	case "chain":
		sql = `
			SELECT id, slug
			FROM chains
			WHERE LOWER(slug) = LOWER($1) OR LOWER(symbol) = LOWER($1) OR LOWER(name) = LOWER($1)
			LIMIT 1
		`
	default:
		return nil, fmt.Errorf("unsupported reference kind %q", kind)
	}

	var ref reference
	err := r.db.QueryRow(ctx, sql, value).Scan(&ref.id, &ref.slug)
	if errorsIsNoRows(err) {
		return nil, UnknownReferenceError{Kind: kind, Value: value}
	}
	if err != nil {
		return nil, fmt.Errorf("resolve %s %q: %w", kind, value, err)
	}
	return &ref, nil
}

func errorsIsNoRows(err error) bool {
	return err == pgx.ErrNoRows
}

func referenceIDArg(ref *reference) any {
	if ref == nil {
		return nil
	}
	return ref.id
}

func referenceSlug(ref *reference) string {
	if ref == nil {
		return ""
	}
	return ref.slug
}

var _ Repository = (*SQLRepository)(nil)
