package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const defaultStatusLimit = 20

type Message struct {
	ChatID          int64
	MessageID       int
	Text            string
	CallbackQueryID string
	CallbackData    string
}

type Replier interface {
	Reply(ctx context.Context, chatID int64, text string) error
}

type Repository interface {
	EnsureUser(ctx context.Context, chatID int64) (User, error)
	CreateRule(ctx context.Context, chatID int64, cmd SubscribeCommand) (Rule, error)
	ListRules(ctx context.Context, chatID int64) ([]Rule, error)
	DeleteRule(ctx context.Context, chatID int64, ruleID int64) (bool, error)
	DeleteRules(ctx context.Context, chatID int64) (int64, error)
	FindRails(ctx context.Context, filter StatusCommand, limit int) ([]RailStatus, error)
}

type Handler struct {
	repo     Repository
	replier  Replier
	mu       sync.Mutex
	sessions map[int64]*demoSession
}

func NewHandler(repo Repository, replier Replier) *Handler {
	return &Handler{
		repo:     repo,
		replier:  replier,
		sessions: make(map[int64]*demoSession),
	}
}

func (h *Handler) Handle(ctx context.Context, msg Message) error {
	if strings.TrimSpace(msg.CallbackData) != "" {
		return h.handleCallback(ctx, msg)
	}

	cmd, err := Parse(msg.Text)
	if errors.Is(err, ErrNotCommand) {
		return nil
	}
	if err != nil {
		return h.reply(ctx, msg.ChatID, fmt.Sprintf("%s\n\n%s", err.Error(), HelpText()))
	}

	switch cmd.Kind {
	case KindStart:
		return h.handleStart(ctx, msg.ChatID)
	case KindHelp:
		return h.reply(ctx, msg.ChatID, HelpText())
	case KindSubscribe:
		return h.handleSubscribe(ctx, msg.ChatID, cmd.Subscribe)
	case KindList:
		return h.handleList(ctx, msg.ChatID)
	case KindUnsubscribe:
		return h.handleUnsubscribe(ctx, msg.ChatID, cmd.RuleID)
	case KindStatus:
		return h.handleStatus(ctx, msg.ChatID, cmd.Status)
	default:
		return h.reply(ctx, msg.ChatID, HelpText())
	}
}

func (h *Handler) handleStart(ctx context.Context, chatID int64) error {
	if h.repo == nil {
		return h.reply(ctx, chatID, "Bot storage is not configured yet.")
	}
	if _, err := h.repo.EnsureUser(ctx, chatID); err != nil {
		if replyErr := h.reply(ctx, chatID, "I could not register this chat right now."); replyErr != nil {
			return replyErr
		}
		return err
	}
	return h.replyMainMenu(ctx, chatID)
}

func (h *Handler) handleSubscribe(ctx context.Context, chatID int64, cmd SubscribeCommand) error {
	if h.repo == nil {
		return h.reply(ctx, chatID, "Bot storage is not configured yet.")
	}
	rule, err := h.repo.CreateRule(ctx, chatID, cmd)
	if err != nil {
		var unknown UnknownReferenceError
		if errors.As(err, &unknown) {
			return h.reply(ctx, chatID, unknown.Error())
		}
		if replyErr := h.reply(ctx, chatID, "I could not create that subscription right now."); replyErr != nil {
			return replyErr
		}
		return err
	}
	return h.reply(ctx, chatID, "Subscribed "+formatRule(rule)+".")
}

func (h *Handler) handleList(ctx context.Context, chatID int64) error {
	if h.repo == nil {
		return h.reply(ctx, chatID, "Bot storage is not configured yet.")
	}
	rules, err := h.repo.ListRules(ctx, chatID)
	if err != nil {
		if replyErr := h.reply(ctx, chatID, "I could not list subscriptions right now."); replyErr != nil {
			return replyErr
		}
		return err
	}
	if len(rules) == 0 {
		return h.replyWithKeyboard(ctx, chatID, strings.Join([]string{
			"No alert filters yet.",
			"",
			"Use the setup button to choose exchanges, then decide whether to watch every rail or narrow by token and chain.",
		}, "\n"), mainMenuKeyboard())
	}

	return h.replyWithKeyboard(ctx, chatID, formatRulesSummary(rules), alertListKeyboard())
}

func (h *Handler) handleUnsubscribe(ctx context.Context, chatID int64, ruleID int64) error {
	if h.repo == nil {
		return h.reply(ctx, chatID, "Bot storage is not configured yet.")
	}
	deleted, err := h.repo.DeleteRule(ctx, chatID, ruleID)
	if err != nil {
		if replyErr := h.reply(ctx, chatID, "I could not delete that subscription right now."); replyErr != nil {
			return replyErr
		}
		return err
	}
	if !deleted {
		return h.reply(ctx, chatID, fmt.Sprintf("Rule #%d was not found for this chat.", ruleID))
	}
	return h.reply(ctx, chatID, fmt.Sprintf("Unsubscribed rule #%d.", ruleID))
}

func (h *Handler) handleStatus(ctx context.Context, chatID int64, filter StatusCommand) error {
	if h.repo == nil {
		return h.reply(ctx, chatID, "Bot storage is not configured yet.")
	}
	rails, err := h.repo.FindRails(ctx, filter, defaultStatusLimit)
	if err != nil {
		if replyErr := h.reply(ctx, chatID, "I could not fetch rail status right now."); replyErr != nil {
			return replyErr
		}
		return err
	}
	if len(rails) == 0 {
		return h.reply(ctx, chatID, "No matching rails found.")
	}

	lines := make([]string, 0, len(rails)+1)
	lines = append(lines, "Current rail status:")
	for _, rail := range rails {
		lines = append(lines, "- "+formatRailStatus(rail))
	}
	if len(rails) == defaultStatusLimit {
		lines = append(lines, fmt.Sprintf("Showing first %d matches. Add filters to narrow this down.", defaultStatusLimit))
	}
	return h.reply(ctx, chatID, strings.Join(lines, "\n"))
}

func (h *Handler) reply(ctx context.Context, chatID int64, text string) error {
	if h.replier == nil {
		return errors.New("telegram replier is not configured")
	}
	return h.replier.Reply(ctx, chatID, text)
}

func HelpText() string {
	return strings.Join([]string{
		"CEX Router alerts watches exchange deposit/withdrawal rails and pings this chat when matching rails change.",
		"",
		"Commands:",
		"/start - open the clickable setup",
		"/list - view saved alert filters",
		"/unsubscribe <rule_id> - remove a filter",
		"/status [exchange] [coin] [chain] - check live rails",
		"/subscribe [exchange] [coin] [chain] [event_types] - advanced manual setup",
		"",
		"Use * as a wildcard, for example: /subscribe okx USDT TRON withdraw_off,withdraw_on",
	}, "\n")
}
