package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/pedro/cex-router/internal/api"
	"github.com/pedro/cex-router/internal/api/eventmeta"
)

const (
	handlerTimeout = 8 * time.Second
	maxListLimit   = 500
)

func chainLookupCondition(alias string, param string) string {
	return fmt.Sprintf(`(
		LOWER(%[1]s.slug) = LOWER(%[2]s)
		OR LOWER(%[1]s.symbol) = LOWER(%[2]s)
		OR LOWER(%[1]s.name) = LOWER(%[2]s)
		OR EXISTS (
			SELECT 1
			FROM chain_aliases route_chain_alias
			WHERE route_chain_alias.chain_id = %[1]s.id
			  AND (
				  LOWER(route_chain_alias.raw_symbol) = LOWER(%[2]s)
				  OR LOWER(route_chain_alias.raw_name) = LOWER(%[2]s)
				  OR LOWER(route_chain_alias.raw_network_id) = LOWER(%[2]s)
			  )
		)
	)`, alias, param)
}

type handler struct {
	db *pgxpool.Pool
}

// Mount attaches the P8 read-only REST endpoints. The main API router wires this
// in after the foundation routes once integration is ready.
func Mount(r chi.Router, deps api.Deps) {
	h := handler{db: deps.DB}

	r.Get("/v1/exchanges", h.listExchanges)
	r.Get("/v1/coins", h.listCoins)
	r.Get("/v1/chains", h.listChains)
	r.Get("/v1/rails", h.listRails)
	r.Get("/v1/route-options", h.listRouteOptions)
	r.Get("/v1/routes", h.listRoutes)
	r.Get("/v1/events", h.listEvents)
}

func (h handler) listExchanges(w http.ResponseWriter, r *http.Request) {
	if !h.requireDB(w) {
		return
	}

	ctx, cancel := withHandlerTimeout(r)
	defer cancel()

	rows, err := h.db.Query(ctx, `
		SELECT e.id, e.slug, e.name, e.region
		FROM exchanges e
		JOIN adapter_freshness af ON af.exchange_id = e.id
		WHERE af.last_successful_poll IS NOT NULL
		   OR af.last_attempt IS NOT NULL
		   OR af.last_error IS NOT NULL
		ORDER BY e.slug
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_query_failed", "failed to list exchanges")
		return
	}
	defer rows.Close()

	items := make([]exchangeListItem, 0)
	for rows.Next() {
		var item exchangeListItem
		if err := rows.Scan(&item.ID, &item.Slug, &item.Name, &item.Region); err != nil {
			writeError(w, http.StatusInternalServerError, "database_scan_failed", "failed to read exchanges")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "database_rows_failed", "failed to list exchanges")
		return
	}

	writeJSON(w, http.StatusOK, listResponse[exchangeListItem]{Data: items})
}

func (h handler) listCoins(w http.ResponseWriter, r *http.Request) {
	if !h.requireDB(w) {
		return
	}

	ctx, cancel := withHandlerTimeout(r)
	defer cancel()

	rows, err := h.db.Query(ctx, `
		SELECT id, slug, symbol, name, external_ids
		FROM coins
		ORDER BY slug
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_query_failed", "failed to list coins")
		return
	}
	defer rows.Close()

	items := make([]coinListItem, 0)
	for rows.Next() {
		var item coinListItem
		var externalIDs []byte
		if err := rows.Scan(&item.ID, &item.Slug, &item.Symbol, &item.Name, &externalIDs); err != nil {
			writeError(w, http.StatusInternalServerError, "database_scan_failed", "failed to read coins")
			return
		}
		if len(externalIDs) == 0 {
			externalIDs = []byte(`{}`)
		}
		item.ExternalIDs = json.RawMessage(externalIDs)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "database_rows_failed", "failed to list coins")
		return
	}

	writeJSON(w, http.StatusOK, listResponse[coinListItem]{Data: items})
}

func (h handler) listChains(w http.ResponseWriter, r *http.Request) {
	if !h.requireDB(w) {
		return
	}

	ctx, cancel := withHandlerTimeout(r)
	defer cancel()

	rows, err := h.db.Query(ctx, `
		SELECT id, slug, symbol, name, evm_chain_id, parent_chain_id
		FROM chains
		ORDER BY slug
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_query_failed", "failed to list chains")
		return
	}
	defer rows.Close()

	items := make([]chainRef, 0)
	for rows.Next() {
		var item chainRef
		var evmChainID pgtype.Int4
		var parentChainID pgtype.Int4
		if err := rows.Scan(&item.ID, &item.Slug, &item.Symbol, &item.Name, &evmChainID, &parentChainID); err != nil {
			writeError(w, http.StatusInternalServerError, "database_scan_failed", "failed to read chains")
			return
		}
		item.EVMChainID = int4Ptr(evmChainID)
		item.ParentChainID = int4Ptr(parentChainID)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "database_rows_failed", "failed to list chains")
		return
	}

	writeJSON(w, http.StatusOK, listResponse[chainRef]{Data: items})
}

func (h handler) listRails(w http.ResponseWriter, r *http.Request) {
	if !h.requireDB(w) {
		return
	}

	limit, err := parseLimit(r, 100, maxListLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}

	query := r.URL.Query()
	depositEnabled, err := optionalBoolParam(query.Get("deposit_enabled"), "deposit_enabled")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	withdrawEnabled, err := optionalBoolParam(query.Get("withdraw_enabled"), "withdraw_enabled")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}

	clauses := []string{"1=1"}
	args := make([]any, 0, 10)
	addFilter := func(sql string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(sql, len(args)))
	}

	if exchange := strings.TrimSpace(query.Get("exchange")); exchange != "" {
		addFilter("e.slug = $%d", exchange)
	}
	if coin := strings.TrimSpace(query.Get("coin")); coin != "" {
		addFilter("c.slug = $%d", coin)
	}
	if chain := strings.TrimSpace(query.Get("chain")); chain != "" {
		addFilter("ch.slug = $%d", chain)
	}
	if depositEnabled != nil {
		addFilter("r.deposit_enabled = $%d", *depositEnabled)
	}
	if withdrawEnabled != nil {
		addFilter("r.withdraw_enabled = $%d", *withdrawEnabled)
	}
	if rawCursor := strings.TrimSpace(query.Get("cursor")); rawCursor != "" {
		cursor, err := decodeRailsCursor(rawCursor)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is not a valid rails cursor")
			return
		}
		start := len(args) + 1
		args = append(args, cursor.CoinID, cursor.ChainID, cursor.ExchangeID)
		clauses = append(clauses, fmt.Sprintf("(r.coin_id, r.chain_id, r.exchange_id) > ($%d, $%d, $%d)", start, start+1, start+2))
	}

	args = append(args, limit+1)
	sql := fmt.Sprintf(`
		SELECT %s
		FROM rails r
		JOIN exchanges e ON e.id = r.exchange_id
		JOIN coins c ON c.id = r.coin_id
		JOIN chains ch ON ch.id = r.chain_id
		WHERE %s
		ORDER BY r.coin_id, r.chain_id, r.exchange_id
		LIMIT $%d
	`, railSelectColumns("r", "e", "c", "ch"), strings.Join(clauses, " AND "), len(args))

	ctx, cancel := withHandlerTimeout(r)
	defer cancel()

	rows, err := h.db.Query(ctx, sql, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_query_failed", "failed to list rails")
		return
	}
	defer rows.Close()

	items := make([]rail, 0, limit)
	for rows.Next() {
		item, err := scanRail(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database_scan_failed", "failed to read rails")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "database_rows_failed", "failed to list rails")
		return
	}

	var nextCursor string
	if len(items) > limit {
		nextCursor, err = encodeRailsCursor(items[limit-1])
		if err != nil {
			writeError(w, http.StatusInternalServerError, "cursor_encode_failed", "failed to encode next cursor")
			return
		}
		items = items[:limit]
	}

	writeJSON(w, http.StatusOK, paginatedResponse[rail]{
		Data:       items,
		NextCursor: nextCursor,
	})
}

func (h handler) listRoutes(w http.ResponseWriter, r *http.Request) {
	if !h.requireDB(w) {
		return
	}

	query := r.URL.Query()
	coin := strings.TrimSpace(query.Get("coin"))
	fromChain := strings.TrimSpace(query.Get("from_chain"))
	toChain := strings.TrimSpace(query.Get("to_chain"))
	if coin == "" || fromChain == "" || toChain == "" {
		writeError(w, http.StatusBadRequest, "missing_route_filter", "coin, from_chain, and to_chain are required")
		return
	}

	var amount decimal.Decimal
	amountProvided := false
	if rawAmount := strings.TrimSpace(query.Get("amount")); rawAmount != "" {
		parsed, err := decimal.NewFromString(rawAmount)
		if err != nil || parsed.IsNegative() {
			writeError(w, http.StatusBadRequest, "invalid_amount", "amount must be a non-negative decimal string")
			return
		}
		amount = parsed
		amountProvided = true
	}
	equivalentAssetsParam, err := optionalBoolParam(query.Get("equivalent_assets"), "equivalent_assets")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_equivalent_assets", err.Error())
		return
	}
	equivalentAssets := equivalentAssetsParam != nil && *equivalentAssetsParam

	coinJoinCondition := routeCoinJoinCondition(equivalentAssets)

	sql := fmt.Sprintf(`
		SELECT %s, %s
		FROM rails d
		JOIN rails w ON %s
		JOIN exchanges e ON e.id = d.exchange_id
		JOIN coins deposit_coin ON deposit_coin.id = d.coin_id
		JOIN coins withdraw_coin ON withdraw_coin.id = w.coin_id
		JOIN chains from_chain ON from_chain.id = d.chain_id
		JOIN chains to_chain ON to_chain.id = w.chain_id
		WHERE deposit_coin.slug = $1
		  AND %s
		  AND %s
		  AND d.is_active = TRUE
		  AND w.is_active = TRUE
		  AND d.deposit_enabled = TRUE
		  AND w.withdraw_enabled = TRUE
		  AND w.withdraw_fee_type IS NOT NULL
	`,
		railSelectColumns("d", "e", "deposit_coin", "from_chain"),
		railSelectColumns("w", "e", "withdraw_coin", "to_chain"),
		coinJoinCondition,
		chainLookupCondition("from_chain", "$2"),
		chainLookupCondition("to_chain", "$3"),
	)

	ctx, cancel := withHandlerTimeout(r)
	defer cancel()

	rows, err := h.db.Query(ctx, sql, coin, fromChain, toChain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_query_failed", "failed to find routes")
		return
	}
	defer rows.Close()

	routes := make([]route, 0)
	requiresAmount := false
	for rows.Next() {
		var depositRail rail
		var withdrawRail rail
		depositNullable := railNullable{}
		withdrawNullable := railNullable{}
		dest := append(railScanDest(&depositRail, &depositNullable), railScanDest(&withdrawRail, &withdrawNullable)...)
		if err := rows.Scan(dest...); err != nil {
			writeError(w, http.StatusInternalServerError, "database_scan_failed", "failed to read routes")
			return
		}
		finishRail(&depositRail, depositNullable)
		finishRail(&withdrawRail, withdrawNullable)

		if feeRequiresAmount(withdrawRail.WithdrawFeeType) {
			requiresAmount = true
		}

		totalFee, err := estimateWithdrawFee(withdrawRail, amount, amountProvided)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "fee_estimate_failed", "failed to estimate route fee")
			return
		}

		equivalentAsset := depositRail.Coin.ID != withdrawRail.Coin.ID
		routeKind := "same_asset"
		if equivalentAsset {
			routeKind = "equivalent_asset"
		}

		routes = append(routes, route{
			Exchange:         depositRail.Exchange,
			Coin:             depositRail.Coin,
			FromCoin:         depositRail.Coin,
			ToCoin:           withdrawRail.Coin,
			FromChain:        depositRail.Chain,
			ToChain:          withdrawRail.Chain,
			DepositRail:      depositRail,
			WithdrawRail:     withdrawRail,
			TotalFeeEstimate: totalFee.String(),
			EquivalentAsset:  equivalentAsset,
			RouteKind:        routeKind,
			totalFee:         totalFee,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "database_rows_failed", "failed to find routes")
		return
	}
	if requiresAmount && !amountProvided {
		writeError(w, http.StatusBadRequest, "amount_required", "amount is required when a candidate route has percent or hybrid withdraw fees")
		return
	}

	sort.SliceStable(routes, func(i, j int) bool {
		if cmp := routes[i].totalFee.Cmp(routes[j].totalFee); cmp != 0 {
			return cmp < 0
		}
		return routes[i].Exchange.Slug < routes[j].Exchange.Slug
	})

	writeJSON(w, http.StatusOK, listResponse[route]{Data: routes})
}

func (h handler) listRouteOptions(w http.ResponseWriter, r *http.Request) {
	if !h.requireDB(w) {
		return
	}

	query := r.URL.Query()
	coin := strings.TrimSpace(query.Get("coin"))
	if coin == "" {
		writeError(w, http.StatusBadRequest, "missing_coin", "coin is required")
		return
	}

	equivalentAssetsParam, err := optionalBoolParam(query.Get("equivalent_assets"), "equivalent_assets")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_equivalent_assets", err.Error())
		return
	}
	equivalentAssets := equivalentAssetsParam != nil && *equivalentAssetsParam

	coinJoinCondition := routeCoinJoinCondition(equivalentAssets)
	sql := fmt.Sprintf(`
		SELECT DISTINCT
		       deposit_coin.id,
		       deposit_coin.slug,
		       deposit_coin.symbol,
		       deposit_coin.name,
		       from_chain.id,
		       from_chain.slug,
		       from_chain.symbol,
		       from_chain.name,
		       from_chain.evm_chain_id,
		       from_chain.parent_chain_id,
		       to_chain.id,
		       to_chain.slug,
		       to_chain.symbol,
		       to_chain.name,
		       to_chain.evm_chain_id,
		       to_chain.parent_chain_id
		FROM rails d
		JOIN rails w ON %s
		JOIN coins deposit_coin ON deposit_coin.id = d.coin_id
		JOIN chains from_chain ON from_chain.id = d.chain_id
		JOIN chains to_chain ON to_chain.id = w.chain_id
		WHERE deposit_coin.slug = $1
		  AND d.is_active = TRUE
		  AND w.is_active = TRUE
		  AND d.deposit_enabled = TRUE
		  AND w.withdraw_enabled = TRUE
		  AND w.withdraw_fee_type IS NOT NULL
		ORDER BY from_chain.slug, to_chain.slug
	`, coinJoinCondition)

	ctx, cancel := withHandlerTimeout(r)
	defer cancel()

	rows, err := h.db.Query(ctx, sql, coin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_query_failed", "failed to list route options")
		return
	}
	defer rows.Close()

	var selectedCoin coinRef
	coinLoaded := false
	fromChainsByID := make(map[int32]chainRef)
	toChainsByID := make(map[int32]chainRef)
	pairSeen := make(map[string]struct{})
	options := routeOptions{
		FromChains: make([]chainRef, 0),
		ToChains:   make([]chainRef, 0),
		Pairs:      make([]routeOptionPair, 0),
	}

	for rows.Next() {
		var pair routeOptionPair
		var coinRef coinRef
		var fromEVMChainID pgtype.Int4
		var fromParentChainID pgtype.Int4
		var toEVMChainID pgtype.Int4
		var toParentChainID pgtype.Int4
		if err := rows.Scan(
			&coinRef.ID,
			&coinRef.Slug,
			&coinRef.Symbol,
			&coinRef.Name,
			&pair.FromChain.ID,
			&pair.FromChain.Slug,
			&pair.FromChain.Symbol,
			&pair.FromChain.Name,
			&fromEVMChainID,
			&fromParentChainID,
			&pair.ToChain.ID,
			&pair.ToChain.Slug,
			&pair.ToChain.Symbol,
			&pair.ToChain.Name,
			&toEVMChainID,
			&toParentChainID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "database_scan_failed", "failed to read route options")
			return
		}
		if !coinLoaded {
			selectedCoin = coinRef
			coinLoaded = true
		}
		pair.FromChain.EVMChainID = int4Ptr(fromEVMChainID)
		pair.FromChain.ParentChainID = int4Ptr(fromParentChainID)
		pair.ToChain.EVMChainID = int4Ptr(toEVMChainID)
		pair.ToChain.ParentChainID = int4Ptr(toParentChainID)

		if _, ok := fromChainsByID[pair.FromChain.ID]; !ok {
			fromChainsByID[pair.FromChain.ID] = pair.FromChain
			options.FromChains = append(options.FromChains, pair.FromChain)
		}
		if _, ok := toChainsByID[pair.ToChain.ID]; !ok {
			toChainsByID[pair.ToChain.ID] = pair.ToChain
			options.ToChains = append(options.ToChains, pair.ToChain)
		}

		pairKey := fmt.Sprintf("%d:%d", pair.FromChain.ID, pair.ToChain.ID)
		if _, ok := pairSeen[pairKey]; ok {
			continue
		}
		pairSeen[pairKey] = struct{}{}
		options.Pairs = append(options.Pairs, pair)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "database_rows_failed", "failed to list route options")
		return
	}
	options.Coin = selectedCoin

	writeJSON(w, http.StatusOK, options)
}

func (h handler) listEvents(w http.ResponseWriter, r *http.Request) {
	if !h.requireDB(w) {
		return
	}

	limit, err := parseLimit(r, 50, maxListLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}

	query := r.URL.Query()
	clauses := []string{"1=1"}
	args := make([]any, 0, 10)
	addFilter := func(sql string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(sql, len(args)))
	}

	if eventType := strings.TrimSpace(query.Get("event_type")); eventType != "" {
		addFilter("re.event_type = $%d", eventType)
	}
	if exchange := strings.TrimSpace(query.Get("exchange")); exchange != "" {
		addFilter("e.slug = $%d", exchange)
	}
	if coin := strings.TrimSpace(query.Get("coin")); coin != "" {
		addFilter("c.slug = $%d", coin)
	}
	if chain := strings.TrimSpace(query.Get("chain")); chain != "" {
		addFilter("ch.slug = $%d", chain)
	}
	if rawCursor := strings.TrimSpace(query.Get("cursor")); rawCursor != "" {
		cursor, err := decodeEventsCursor(rawCursor)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is not a valid events cursor")
			return
		}
		start := len(args) + 1
		args = append(args, cursor.OccurredAt, cursor.ID)
		clauses = append(clauses, fmt.Sprintf("(re.occurred_at, re.id) < ($%d, $%d)", start, start+1))
	}

	args = append(args, limit+1)
	sql := fmt.Sprintf(`
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
		ORDER BY re.occurred_at DESC, re.id DESC
		LIMIT $%d
	`, strings.Join(clauses, " AND "), len(args))

	ctx, cancel := withHandlerTimeout(r)
	defer cancel()

	rows, err := h.db.Query(ctx, sql, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_query_failed", "failed to list events")
		return
	}
	defer rows.Close()

	items := make([]event, 0, limit)
	for rows.Next() {
		var item event
		var evmChainID pgtype.Int4
		var parentChainID pgtype.Int4
		var before []byte
		var after []byte
		if err := rows.Scan(
			&item.ID,
			&item.RailID,
			&item.EventType,
			&item.Exchange.ID,
			&item.Exchange.Slug,
			&item.Exchange.Name,
			&item.Exchange.Region,
			&item.Coin.ID,
			&item.Coin.Slug,
			&item.Coin.Symbol,
			&item.Coin.Name,
			&item.Chain.ID,
			&item.Chain.Slug,
			&item.Chain.Symbol,
			&item.Chain.Name,
			&evmChainID,
			&parentChainID,
			&before,
			&after,
			&item.OccurredAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "database_scan_failed", "failed to read events")
			return
		}
		item.Chain.EVMChainID = int4Ptr(evmChainID)
		item.Chain.ParentChainID = int4Ptr(parentChainID)
		item.Before = json.RawMessage(defaultJSONObject(before))
		item.After = json.RawMessage(defaultJSONObject(after))
		details := eventmeta.Build(item.EventType, item.Before, item.After)
		item.Summary = details.Summary
		item.Changes = details.Changes
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "database_rows_failed", "failed to list events")
		return
	}

	var nextCursor string
	if len(items) > limit {
		nextCursor, err = encodeEventsCursor(items[limit-1])
		if err != nil {
			writeError(w, http.StatusInternalServerError, "cursor_encode_failed", "failed to encode next cursor")
			return
		}
		items = items[:limit]
	}

	writeJSON(w, http.StatusOK, paginatedResponse[event]{
		Data:       items,
		NextCursor: nextCursor,
	})
}

func (h handler) requireDB(w http.ResponseWriter) bool {
	if h.db != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, "database_unavailable", "database pool is not configured")
	return false
}

func withHandlerTimeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), handlerTimeout)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, errorResponse{
		Error: responseError{
			Code:    code,
			Message: message,
		},
	})
}

func parseLimit(r *http.Request, defaultLimit int, maxLimit int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit < 1 {
		return 0, fmt.Errorf("limit must be at least 1")
	}
	if limit > maxLimit {
		return 0, fmt.Errorf("limit must be at most %d", maxLimit)
	}
	return limit, nil
}

func optionalBoolParam(raw string, name string) (*bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be a boolean", name)
	}
	return &value, nil
}

func routeCoinJoinCondition(equivalentAssets bool) string {
	if !equivalentAssets {
		return "w.exchange_id = d.exchange_id AND w.coin_id = d.coin_id"
	}
	return `
		w.exchange_id = d.exchange_id
		AND (
			w.coin_id = d.coin_id
			OR EXISTS (
				SELECT 1
				FROM asset_family_members deposit_member
				JOIN asset_family_members withdraw_member
				  ON withdraw_member.family_id = deposit_member.family_id
				WHERE deposit_member.coin_id = d.coin_id
				  AND withdraw_member.coin_id = w.coin_id
			)
		)
	`
}

func defaultJSONObject(value []byte) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	return value
}

func int4Ptr(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

type railNullable struct {
	depositConfirmations pgtype.Int4
	withdrawMin          pgtype.Text
	withdrawFee          pgtype.Text
	withdrawFeeType      pgtype.Text
	withdrawFeePercent   pgtype.Text
	depositOffStartedAt  pgtype.Timestamptz
	withdrawOffStartedAt pgtype.Timestamptz
	missingSince         pgtype.Timestamptz
	evmChainID           pgtype.Int4
	parentChainID        pgtype.Int4
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRail(scanner rowScanner) (rail, error) {
	item := rail{}
	nullable := railNullable{}
	if err := scanner.Scan(railScanDest(&item, &nullable)...); err != nil {
		return rail{}, err
	}
	finishRail(&item, nullable)
	return item, nil
}

func railScanDest(item *rail, nullable *railNullable) []any {
	return []any{
		&item.ID,
		&item.Exchange.ID,
		&item.Exchange.Slug,
		&item.Exchange.Name,
		&item.Exchange.Region,
		&item.Coin.ID,
		&item.Coin.Slug,
		&item.Coin.Symbol,
		&item.Coin.Name,
		&item.Chain.ID,
		&item.Chain.Slug,
		&item.Chain.Symbol,
		&item.Chain.Name,
		&nullable.evmChainID,
		&nullable.parentChainID,
		&item.DepositEnabled,
		&item.WithdrawEnabled,
		&nullable.depositConfirmations,
		&nullable.withdrawMin,
		&nullable.withdrawFee,
		&nullable.withdrawFeeType,
		&nullable.withdrawFeePercent,
		&nullable.depositOffStartedAt,
		&nullable.withdrawOffStartedAt,
		&item.IsActive,
		&nullable.missingSince,
		&item.MissingCount,
		&item.IsInitial,
		&item.LastSeenAt,
	}
}

func finishRail(item *rail, nullable railNullable) {
	item.Chain.EVMChainID = int4Ptr(nullable.evmChainID)
	item.Chain.ParentChainID = int4Ptr(nullable.parentChainID)
	item.DepositConfirmations = int4Ptr(nullable.depositConfirmations)
	item.WithdrawMin = textPtr(nullable.withdrawMin)
	item.WithdrawFee = textPtr(nullable.withdrawFee)
	item.WithdrawFeeType = textPtr(nullable.withdrawFeeType)
	item.WithdrawFeePercent = textPtr(nullable.withdrawFeePercent)
	item.DepositOffStartedAt = timestamptzPtr(nullable.depositOffStartedAt)
	item.WithdrawOffStartedAt = timestamptzPtr(nullable.withdrawOffStartedAt)
	item.MissingSince = timestamptzPtr(nullable.missingSince)
}

func railSelectColumns(railAlias string, exchangeAlias string, coinAlias string, chainAlias string) string {
	return strings.Join([]string{
		railAlias + ".id",
		exchangeAlias + ".id",
		exchangeAlias + ".slug",
		exchangeAlias + ".name",
		exchangeAlias + ".region",
		coinAlias + ".id",
		coinAlias + ".slug",
		coinAlias + ".symbol",
		coinAlias + ".name",
		chainAlias + ".id",
		chainAlias + ".slug",
		chainAlias + ".symbol",
		chainAlias + ".name",
		chainAlias + ".evm_chain_id",
		chainAlias + ".parent_chain_id",
		railAlias + ".deposit_enabled",
		railAlias + ".withdraw_enabled",
		railAlias + ".deposit_confirmations",
		"(" + railAlias + ".withdraw_min)::text",
		"(" + railAlias + ".withdraw_fee)::text",
		railAlias + ".withdraw_fee_type",
		"(" + railAlias + ".withdraw_fee_percent)::text",
		railAlias + ".deposit_off_started_at",
		railAlias + ".withdraw_off_started_at",
		railAlias + ".is_active",
		railAlias + ".missing_since",
		railAlias + ".missing_count",
		railAlias + ".is_initial",
		railAlias + ".last_seen_at",
	}, ", ")
}

func feeRequiresAmount(feeType *string) bool {
	if feeType == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(*feeType)) {
	case "percent", "hybrid":
		return true
	default:
		return false
	}
}

func estimateWithdrawFee(item rail, amount decimal.Decimal, amountProvided bool) (decimal.Decimal, error) {
	fixed, err := decimalFromOptionalString(item.WithdrawFee)
	if err != nil {
		return decimal.Zero, err
	}
	percent, err := decimalFromOptionalString(item.WithdrawFeePercent)
	if err != nil {
		return decimal.Zero, err
	}

	if item.WithdrawFeeType == nil {
		return fixed, nil
	}

	switch strings.ToLower(strings.TrimSpace(*item.WithdrawFeeType)) {
	case "", "fixed":
		return fixed, nil
	case "percent":
		if !amountProvided {
			return decimal.Zero, nil
		}
		return amount.Mul(percent).Div(decimal.NewFromInt(100)), nil
	case "hybrid":
		if !amountProvided {
			return fixed, nil
		}
		return fixed.Add(amount.Mul(percent).Div(decimal.NewFromInt(100))), nil
	default:
		return fixed, nil
	}
}

func decimalFromOptionalString(raw *string) (decimal.Decimal, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return decimal.Zero, nil
	}
	value, err := decimal.NewFromString(*raw)
	if err != nil {
		return decimal.Zero, err
	}
	return value, nil
}
