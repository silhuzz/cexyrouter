package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const callbackPrefix = "demo:"

type demoChoice struct {
	Slug  string
	Label string
}

var demoExchangeOptions = []demoChoice{
	{Slug: "binance", Label: "Binance"},
	{Slug: "bybit", Label: "Bybit"},
	{Slug: "okx", Label: "OKX"},
	{Slug: "bithumb", Label: "Bithumb"},
	{Slug: "bitget", Label: "Bitget"},
	{Slug: "kucoin", Label: "KuCoin"},
	{Slug: "gate", Label: "Gate.io"},
	{Slug: "htx", Label: "HTX"},
	{Slug: "coinex", Label: "CoinEx"},
	{Slug: "whitebit", Label: "WhiteBIT"},
	{Slug: "bitmart", Label: "BitMart"},
}

var demoTokenOptions = []demoChoice{
	{Slug: "usdt", Label: "USDT"},
	{Slug: "usdc", Label: "USDC"},
	{Slug: "btc", Label: "BTC"},
	{Slug: "eth", Label: "ETH"},
	{Slug: "xrp", Label: "XRP"},
}

var demoChainOptions = []demoChoice{
	{Slug: "ethereum", Label: "Ethereum"},
	{Slug: "tron", Label: "TRON"},
	{Slug: "bsc", Label: "BSC"},
	{Slug: "solana", Label: "Solana"},
	{Slug: "polygon", Label: "Polygon"},
	{Slug: "arbitrum", Label: "Arbitrum"},
	{Slug: "base", Label: "Base"},
	{Slug: "bitcoin", Label: "Bitcoin"},
}

type demoSession struct {
	AllExchanges bool
	Exchanges    map[string]bool
	AllTokens    bool
	Tokens       map[string]bool
	AllChains    bool
	Chains       map[string]bool
}

type demoSelection struct {
	AllExchanges bool
	Exchanges    map[string]bool
	AllTokens    bool
	Tokens       map[string]bool
	AllChains    bool
	Chains       map[string]bool
}

func (h *Handler) handleCallback(ctx context.Context, msg Message) error {
	action := strings.TrimSpace(msg.CallbackData)
	if !strings.HasPrefix(action, callbackPrefix) {
		h.answerCallback(ctx, msg, "Unknown action")
		return nil
	}
	action = strings.TrimPrefix(action, callbackPrefix)

	switch {
	case action == "start":
		h.answerCallback(ctx, msg, "")
		return h.showExchangePicker(ctx, msg, h.resetDemoSession(msg.ChatID))
	case action == "list":
		h.answerCallback(ctx, msg, "")
		return h.handleList(ctx, msg.ChatID)
	case action == "status":
		h.answerCallback(ctx, msg, "")
		return h.handleStatus(ctx, msg.ChatID, StatusCommand{})
	case action == "help":
		h.answerCallback(ctx, msg, "")
		return h.reply(ctx, msg.ChatID, HelpText())
	case action == "reset_confirm":
		h.answerCallback(ctx, msg, "")
		return h.showResetConfirmation(ctx, msg)
	case action == "reset":
		h.answerCallback(ctx, msg, "")
		return h.resetAlertSettings(ctx, msg)
	case action == "cancel":
		h.answerCallback(ctx, msg, "")
		h.deleteDemoSession(msg.ChatID)
		return h.editOrReplyKeyboard(ctx, msg, "Setup cancelled.\n\nNo alert filters were changed.", mainMenuKeyboard())
	case action == "back_ex":
		h.answerCallback(ctx, msg, "")
		return h.showExchangePicker(ctx, msg, h.snapshotDemoSession(msg.ChatID))
	case action == "back_tok":
		h.answerCallback(ctx, msg, "")
		return h.showTokenPicker(ctx, msg, h.snapshotDemoSession(msg.ChatID))
	case strings.HasPrefix(action, "ex:"):
		h.answerCallback(ctx, msg, "")
		return h.showExchangePicker(ctx, msg, h.toggleDemoExchange(msg.ChatID, strings.TrimPrefix(action, "ex:")))
	case action == "ex_next":
		selection := h.snapshotDemoSession(msg.ChatID)
		if !selection.hasExchange() {
			h.answerCallback(ctx, msg, "Pick at least one exchange first")
			return nil
		}
		h.answerCallback(ctx, msg, "")
		return h.showScopePicker(ctx, msg, selection)
	case action == "scope:all":
		selection := h.snapshotDemoSession(msg.ChatID)
		if !selection.hasExchange() {
			h.answerCallback(ctx, msg, "Pick at least one exchange first")
			return nil
		}
		h.answerCallback(ctx, msg, "")
		selection.AllTokens = true
		selection.Tokens = map[string]bool{}
		selection.AllChains = true
		selection.Chains = map[string]bool{}
		return h.saveDemoRules(ctx, msg, selection)
	case action == "scope:custom":
		selection := h.ensureCustomDemoSession(msg.ChatID)
		if !selection.hasExchange() {
			h.answerCallback(ctx, msg, "Pick at least one exchange first")
			return nil
		}
		h.answerCallback(ctx, msg, "")
		return h.showTokenPicker(ctx, msg, selection)
	case strings.HasPrefix(action, "tok:"):
		h.answerCallback(ctx, msg, "")
		return h.showTokenPicker(ctx, msg, h.toggleDemoToken(msg.ChatID, strings.TrimPrefix(action, "tok:")))
	case action == "tok_next":
		selection := h.snapshotDemoSession(msg.ChatID)
		if !selection.hasExchange() {
			h.answerCallback(ctx, msg, "Pick at least one exchange first")
			return nil
		}
		h.answerCallback(ctx, msg, "")
		return h.showChainPicker(ctx, msg, selection)
	case strings.HasPrefix(action, "ch:"):
		h.answerCallback(ctx, msg, "")
		return h.showChainPicker(ctx, msg, h.toggleDemoChain(msg.ChatID, strings.TrimPrefix(action, "ch:")))
	case action == "save":
		selection := h.snapshotDemoSession(msg.ChatID)
		if !selection.hasExchange() {
			h.answerCallback(ctx, msg, "Pick at least one exchange first")
			return nil
		}
		h.answerCallback(ctx, msg, "")
		return h.saveDemoRules(ctx, msg, selection)
	default:
		h.answerCallback(ctx, msg, "Unknown action")
		return h.replyWithKeyboard(ctx, msg.ChatID, "That button is stale. Start again?", mainMenuKeyboard())
	}
}

func (h *Handler) replyMainMenu(ctx context.Context, chatID int64) error {
	return h.replyWithKeyboard(ctx, chatID, strings.Join([]string{
		"CEX Router alerts",
		"",
		"Get Telegram pings when exchange deposit or withdrawal rails change.",
		"",
		"Use this chat to choose the exchanges you care about, then watch every token/chain on those exchanges or narrow alerts to a short list.",
		"",
		"Alerts cover rail on/off changes, delist/relist events, fee changes, and minimum withdrawal changes.",
	}, "\n"), mainMenuKeyboard())
}

func mainMenuKeyboard() Keyboard {
	return Keyboard{
		{{Text: "Set up alerts", Data: callbackPrefix + "start"}},
		{
			{Text: "View my alerts", Data: callbackPrefix + "list"},
			{Text: "Live rail status", Data: callbackPrefix + "status"},
		},
		{{Text: "Reset alert settings", Data: callbackPrefix + "reset_confirm"}},
		{{Text: "Command help", Data: callbackPrefix + "help"}},
	}
}

func alertListKeyboard() Keyboard {
	return Keyboard{
		{{Text: "Add another filter", Data: callbackPrefix + "start"}},
		{{Text: "Reset alert settings", Data: callbackPrefix + "reset_confirm"}},
	}
}

func (h *Handler) showExchangePicker(ctx context.Context, msg Message, selection demoSelection) error {
	text := strings.Join([]string{
		"Step 1 of 3: choose exchanges",
		"",
		"Selected: " + selection.exchangeSummary(),
		"",
		"Tap one or more exchanges. For the demo, this is the audience-friendly question: which venues should wake you up when rails move?",
	}, "\n")
	keyboard := choiceRows(demoExchangeOptions, selection.Exchanges, selection.AllExchanges, "ex:")
	keyboard = append(keyboard,
		[]Button{{Text: "Continue to coverage", Data: callbackPrefix + "ex_next"}},
		[]Button{{Text: "Cancel", Data: callbackPrefix + "cancel"}},
	)
	return h.editOrReplyKeyboard(ctx, msg, text, keyboard)
}

func (h *Handler) showScopePicker(ctx context.Context, msg Message, selection demoSelection) error {
	text := strings.Join([]string{
		"Step 2 of 3: choose coverage",
		"",
		"Exchanges: " + selection.exchangeSummary(),
		"",
		"Watch every route-availability rail on those exchanges, or narrow the feed to specific assets and networks.",
	}, "\n")
	keyboard := Keyboard{
		{{Text: "Watch everything", Data: callbackPrefix + "scope:all"}},
		{{Text: "Pick tokens and chains", Data: callbackPrefix + "scope:custom"}},
		{{Text: "Back", Data: callbackPrefix + "back_ex"}, {Text: "Cancel", Data: callbackPrefix + "cancel"}},
	}
	return h.editOrReplyKeyboard(ctx, msg, text, keyboard)
}

func (h *Handler) showTokenPicker(ctx context.Context, msg Message, selection demoSelection) error {
	text := strings.Join([]string{
		"Step 3 of 3: pick tokens",
		"",
		"Exchanges: " + selection.exchangeSummary(),
		"Tokens: " + selection.tokenSummary(),
		"",
		"Keep Any token selected for broad monitoring, or tap specific assets for a focused feed.",
	}, "\n")
	keyboard := Keyboard{
		{{Text: checkboxLabel(selection.AllTokens, "Any token"), Data: callbackPrefix + "tok:any"}},
	}
	keyboard = append(keyboard, choiceRows(demoTokenOptions, selection.Tokens, selection.AllTokens, "tok:")...)
	keyboard = append(keyboard,
		[]Button{{Text: "Continue to chains", Data: callbackPrefix + "tok_next"}},
		[]Button{{Text: "Back", Data: callbackPrefix + "back_ex"}, {Text: "Cancel", Data: callbackPrefix + "cancel"}},
	)
	return h.editOrReplyKeyboard(ctx, msg, text, keyboard)
}

func (h *Handler) showChainPicker(ctx context.Context, msg Message, selection demoSelection) error {
	text := strings.Join([]string{
		"Final check: pick chains",
		"",
		"Exchanges: " + selection.exchangeSummary(),
		"Tokens: " + selection.tokenSummary(),
		"Chains: " + selection.chainSummary(),
		"",
		"Keep Any chain selected to watch every network, or choose the chains that matter for the route you are demoing.",
	}, "\n")
	keyboard := Keyboard{
		{{Text: checkboxLabel(selection.AllChains, "Any chain"), Data: callbackPrefix + "ch:any"}},
	}
	keyboard = append(keyboard, choiceRows(demoChainOptions, selection.Chains, selection.AllChains, "ch:")...)
	keyboard = append(keyboard,
		[]Button{{Text: "Save alert filters", Data: callbackPrefix + "save"}},
		[]Button{{Text: "Back", Data: callbackPrefix + "back_tok"}, {Text: "Cancel", Data: callbackPrefix + "cancel"}},
	)
	return h.editOrReplyKeyboard(ctx, msg, text, keyboard)
}

func (h *Handler) saveDemoRules(ctx context.Context, msg Message, selection demoSelection) error {
	if h.repo == nil {
		return h.reply(ctx, msg.ChatID, "Bot storage is not configured yet.")
	}

	cmds := buildDemoSubscribeCommands(selection)
	if len(cmds) == 0 {
		return h.replyWithKeyboard(ctx, msg.ChatID, "Pick at least one exchange before saving.", mainMenuKeyboard())
	}

	saved := 0
	for _, cmd := range cmds {
		if _, err := h.repo.CreateRule(ctx, msg.ChatID, cmd); err != nil {
			var unknown UnknownReferenceError
			if errors.As(err, &unknown) {
				return h.editOrReplyKeyboard(ctx, msg, strings.Join([]string{
					"I could not save that filter yet.",
					"",
					fmt.Sprintf("The selected %s %q is not in the current alert database.", unknown.Kind, unknown.Value),
					"",
					"Choose a live exchange, token, or chain and try again.",
				}, "\n"), mainMenuKeyboard())
			}
			if saved > 0 {
				return fmt.Errorf("create alert rule after %d saved: %w", saved, err)
			}
			return fmt.Errorf("create alert rule: %w", err)
		}
		saved++
	}
	h.deleteDemoSession(msg.ChatID)

	text := strings.Join([]string{
		fmt.Sprintf("Saved %d alert %s", saved, plural(saved, "rule")),
		"",
		"Exchanges: " + selection.exchangeSummary(),
		"Tokens: " + selection.tokenSummary(),
		"Chains: " + selection.chainSummary(),
		"Events: route availability changes",
		"",
		"This chat will now get notified when a matching deposit or withdrawal rail turns on/off, delists, or relists.",
	}, "\n")
	keyboard := Keyboard{
		{{Text: "View my alerts", Data: callbackPrefix + "list"}},
		{{Text: "Add another filter", Data: callbackPrefix + "start"}},
	}
	return h.editOrReplyKeyboard(ctx, msg, text, keyboard)
}

func (h *Handler) showResetConfirmation(ctx context.Context, msg Message) error {
	text := strings.Join([]string{
		"Reset alert settings?",
		"",
		"This removes all saved alert filters for this chat and clears any queued unsent Telegram alerts.",
		"",
		"Already-sent alerts and rail history stay in the database.",
	}, "\n")
	keyboard := Keyboard{
		{{Text: "Yes, reset everything", Data: callbackPrefix + "reset"}},
		{{Text: "Keep my alerts", Data: callbackPrefix + "list"}},
	}
	return h.editOrReplyKeyboard(ctx, msg, text, keyboard)
}

func (h *Handler) resetAlertSettings(ctx context.Context, msg Message) error {
	if h.repo == nil {
		return h.reply(ctx, msg.ChatID, "Bot storage is not configured yet.")
	}
	deleted, err := h.repo.DeleteRules(ctx, msg.ChatID)
	if err != nil {
		return fmt.Errorf("reset alert settings: %w", err)
	}
	h.deleteDemoSession(msg.ChatID)

	text := fmt.Sprintf("Reset complete.\n\nRemoved %d saved alert %s and cleared queued unsent alerts for this chat.", deleted, plural(int(deleted), "filter"))
	return h.editOrReplyKeyboard(ctx, msg, text, mainMenuKeyboard())
}

func buildDemoSubscribeCommands(selection demoSelection) []SubscribeCommand {
	exchanges := selectedSlugs(selection.AllExchanges, selection.Exchanges, demoExchangeOptions)
	tokens := selectedSlugs(selection.AllTokens, selection.Tokens, demoTokenOptions)
	chains := selectedSlugs(selection.AllChains, selection.Chains, demoChainOptions)
	if len(exchanges) == 0 {
		return nil
	}
	if len(tokens) == 0 {
		tokens = []string{""}
	}
	if len(chains) == 0 {
		chains = []string{""}
	}

	cmds := make([]SubscribeCommand, 0, len(exchanges)*len(tokens)*len(chains))
	for _, exchange := range exchanges {
		for _, token := range tokens {
			for _, chain := range chains {
				cmds = append(cmds, SubscribeCommand{
					Exchange:   exchange,
					Coin:       token,
					Chain:      chain,
					EventTypes: AvailabilityEventTypes(),
				})
			}
		}
	}
	return cmds
}

func (h *Handler) resetDemoSession(chatID int64) demoSelection {
	h.mu.Lock()
	defer h.mu.Unlock()
	session := newDemoSession()
	h.sessions[chatID] = session
	return session.snapshot()
}

func (h *Handler) snapshotDemoSession(chatID int64) demoSelection {
	h.mu.Lock()
	defer h.mu.Unlock()
	session := h.demoSessionLocked(chatID)
	return session.snapshot()
}

func (h *Handler) ensureCustomDemoSession(chatID int64) demoSelection {
	h.mu.Lock()
	defer h.mu.Unlock()
	session := h.demoSessionLocked(chatID)
	if len(session.Tokens) == 0 {
		session.AllTokens = true
	}
	if len(session.Chains) == 0 {
		session.AllChains = true
	}
	return session.snapshot()
}

func (h *Handler) toggleDemoExchange(chatID int64, slug string) demoSelection {
	h.mu.Lock()
	defer h.mu.Unlock()
	session := h.demoSessionLocked(chatID)
	session.AllExchanges = false
	toggleSet(session.Exchanges, slug)
	return session.snapshot()
}

func (h *Handler) toggleDemoToken(chatID int64, slug string) demoSelection {
	h.mu.Lock()
	defer h.mu.Unlock()
	session := h.demoSessionLocked(chatID)
	if slug == "any" {
		session.AllTokens = true
		session.Tokens = map[string]bool{}
		return session.snapshot()
	}
	session.AllTokens = false
	toggleSet(session.Tokens, slug)
	if len(session.Tokens) == 0 {
		session.AllTokens = true
	}
	return session.snapshot()
}

func (h *Handler) toggleDemoChain(chatID int64, slug string) demoSelection {
	h.mu.Lock()
	defer h.mu.Unlock()
	session := h.demoSessionLocked(chatID)
	if slug == "any" {
		session.AllChains = true
		session.Chains = map[string]bool{}
		return session.snapshot()
	}
	session.AllChains = false
	toggleSet(session.Chains, slug)
	if len(session.Chains) == 0 {
		session.AllChains = true
	}
	return session.snapshot()
}

func (h *Handler) deleteDemoSession(chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, chatID)
}

func (h *Handler) demoSessionLocked(chatID int64) *demoSession {
	session := h.sessions[chatID]
	if session == nil {
		session = newDemoSession()
		h.sessions[chatID] = session
	}
	return session
}

func newDemoSession() *demoSession {
	return &demoSession{
		Exchanges: map[string]bool{},
		AllTokens: true,
		Tokens:    map[string]bool{},
		AllChains: true,
		Chains:    map[string]bool{},
	}
}

func (s *demoSession) snapshot() demoSelection {
	return demoSelection{
		AllExchanges: s.AllExchanges,
		Exchanges:    cloneSet(s.Exchanges),
		AllTokens:    s.AllTokens,
		Tokens:       cloneSet(s.Tokens),
		AllChains:    s.AllChains,
		Chains:       cloneSet(s.Chains),
	}
}

func (s demoSelection) hasExchange() bool {
	return s.AllExchanges || len(s.Exchanges) > 0
}

func (s demoSelection) exchangeSummary() string {
	return summaryValue(s.AllExchanges, s.Exchanges, demoExchangeOptions, "all exchanges", "none selected")
}

func (s demoSelection) tokenSummary() string {
	return summaryValue(s.AllTokens, s.Tokens, demoTokenOptions, "any token", "any token")
}

func (s demoSelection) chainSummary() string {
	return summaryValue(s.AllChains, s.Chains, demoChainOptions, "any chain", "any chain")
}

func (h *Handler) replyWithKeyboard(ctx context.Context, chatID int64, text string, keyboard Keyboard) error {
	if replier, ok := h.replier.(KeyboardReplier); ok {
		return replier.ReplyWithKeyboard(ctx, chatID, text, keyboard)
	}
	return h.reply(ctx, chatID, text)
}

func (h *Handler) editOrReplyKeyboard(ctx context.Context, msg Message, text string, keyboard Keyboard) error {
	if msg.MessageID > 0 {
		if editor, ok := h.replier.(MessageEditor); ok {
			if err := editor.EditMessage(ctx, msg.ChatID, msg.MessageID, text, keyboard); err == nil {
				return nil
			}
		}
	}
	return h.replyWithKeyboard(ctx, msg.ChatID, text, keyboard)
}

func (h *Handler) answerCallback(ctx context.Context, msg Message, text string) {
	if msg.CallbackQueryID == "" {
		return
	}
	if acknowledger, ok := h.replier.(CallbackAcknowledger); ok {
		_ = acknowledger.AnswerCallback(ctx, msg.CallbackQueryID, text)
	}
}

func choiceRows(options []demoChoice, selected map[string]bool, allSelected bool, actionPrefix string) Keyboard {
	rows := make(Keyboard, 0, (len(options)+1)/2)
	for i := 0; i < len(options); i += 2 {
		row := []Button{{
			Text: checkboxLabel(!allSelected && selected[options[i].Slug], options[i].Label),
			Data: callbackPrefix + actionPrefix + options[i].Slug,
		}}
		if i+1 < len(options) {
			row = append(row, Button{
				Text: checkboxLabel(!allSelected && selected[options[i+1].Slug], options[i+1].Label),
				Data: callbackPrefix + actionPrefix + options[i+1].Slug,
			})
		}
		rows = append(rows, row)
	}
	return rows
}

func checkboxLabel(checked bool, label string) string {
	if checked {
		return "[x] " + label
	}
	return "[ ] " + label
}

func summaryValue(all bool, selected map[string]bool, options []demoChoice, allLabel string, emptyLabel string) string {
	if all {
		return allLabel
	}
	labels := selectedLabels(selected, options)
	if len(labels) == 0 {
		return emptyLabel
	}
	return strings.Join(labels, ", ")
}

func selectedLabels(selected map[string]bool, options []demoChoice) []string {
	labelsBySlug := make(map[string]string, len(options))
	for _, option := range options {
		labelsBySlug[option.Slug] = option.Label
	}
	slugs := selectedSlugs(false, selected, options)
	labels := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		label := labelsBySlug[slug]
		if label == "" {
			label = slug
		}
		labels = append(labels, label)
	}
	return labels
}

func selectedSlugs(all bool, selected map[string]bool, options []demoChoice) []string {
	if all {
		return []string{""}
	}

	slugs := make([]string, 0, len(selected))
	for _, option := range options {
		if selected[option.Slug] {
			slugs = append(slugs, option.Slug)
		}
	}
	return slugs
}

func cloneSet(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for key, value := range in {
		if value {
			out[key] = true
		}
	}
	return out
}

func toggleSet(set map[string]bool, value string) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return
	}
	if set[value] {
		delete(set, value)
		return
	}
	set[value] = true
}

func plural(count int, singular string) string {
	if count == 1 {
		return singular
	}
	return singular + "s"
}
