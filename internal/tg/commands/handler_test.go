package commands

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestHandlerUnsubscribeScopesDeleteToChat(t *testing.T) {
	repo := &fakeRepository{deleteResult: true}
	replier := &fakeReplier{}
	handler := NewHandler(repo, replier)

	if err := handler.Handle(context.Background(), Message{
		ChatID: 99,
		Text:   "/unsubscribe 42",
	}); err != nil {
		t.Fatalf("handle unsubscribe: %v", err)
	}

	if repo.deletedChatID != 99 || repo.deletedRuleID != 42 {
		t.Fatalf("delete called with chat=%d rule=%d, want chat=99 rule=42", repo.deletedChatID, repo.deletedRuleID)
	}
	if replier.chatID != 99 {
		t.Fatalf("reply chat = %d, want 99", replier.chatID)
	}
}

func TestHandlerStartShowsButtonSetup(t *testing.T) {
	repo := &fakeRepository{}
	replier := &fakeReplier{}
	handler := NewHandler(repo, replier)

	if err := handler.Handle(context.Background(), Message{
		ChatID: 99,
		Text:   "/start",
	}); err != nil {
		t.Fatalf("handle start: %v", err)
	}

	if replier.chatID != 99 {
		t.Fatalf("reply chat = %d, want 99", replier.chatID)
	}
	if len(replier.keyboard) == 0 || replier.keyboard[0][0].Data != "demo:start" {
		t.Fatalf("start keyboard = %#v, want setup button", replier.keyboard)
	}
}

func TestHandlerCallbackSavesAllExchangeRules(t *testing.T) {
	repo := &fakeRepository{}
	replier := &fakeReplier{}
	handler := NewHandler(repo, replier)
	ctx := context.Background()

	steps := []Message{
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb1", CallbackData: "demo:start"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb2", CallbackData: "demo:ex:okx"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb3", CallbackData: "demo:ex:bitget"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb4", CallbackData: "demo:ex_next"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb5", CallbackData: "demo:scope:all"},
	}
	for _, step := range steps {
		if err := handler.Handle(ctx, step); err != nil {
			t.Fatalf("handle callback %s: %v", step.CallbackData, err)
		}
	}

	want := []SubscribeCommand{
		{Exchange: "okx", EventTypes: AvailabilityEventTypes()},
		{Exchange: "bitget", EventTypes: AvailabilityEventTypes()},
	}
	if !reflect.DeepEqual(repo.created, want) {
		t.Fatalf("created rules = %#v, want %#v", repo.created, want)
	}
	if len(replier.answeredCallbacks) != len(steps) {
		t.Fatalf("answered callbacks = %#v, want %d", replier.answeredCallbacks, len(steps))
	}
}

func TestHandlerCallbackSavesTokenChainSubset(t *testing.T) {
	repo := &fakeRepository{}
	replier := &fakeReplier{}
	handler := NewHandler(repo, replier)
	ctx := context.Background()

	steps := []Message{
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb1", CallbackData: "demo:start"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb2", CallbackData: "demo:ex:okx"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb3", CallbackData: "demo:ex_next"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb4", CallbackData: "demo:scope:custom"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb5", CallbackData: "demo:tok:usdt"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb6", CallbackData: "demo:tok:usdc"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb7", CallbackData: "demo:tok_next"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb8", CallbackData: "demo:ch:tron"},
		{ChatID: 99, MessageID: 10, CallbackQueryID: "cb9", CallbackData: "demo:save"},
	}
	for _, step := range steps {
		if err := handler.Handle(ctx, step); err != nil {
			t.Fatalf("handle callback %s: %v", step.CallbackData, err)
		}
	}

	want := []SubscribeCommand{
		{Exchange: "okx", Coin: "usdt", Chain: "tron", EventTypes: AvailabilityEventTypes()},
		{Exchange: "okx", Coin: "usdc", Chain: "tron", EventTypes: AvailabilityEventTypes()},
	}
	if !reflect.DeepEqual(repo.created, want) {
		t.Fatalf("created rules = %#v, want %#v", repo.created, want)
	}
}

func TestHandlerCallbackResetDeletesRules(t *testing.T) {
	repo := &fakeRepository{deleteRulesResult: 17}
	replier := &fakeReplier{}
	handler := NewHandler(repo, replier)

	if err := handler.Handle(context.Background(), Message{
		ChatID:          99,
		MessageID:       10,
		CallbackQueryID: "cb1",
		CallbackData:    "demo:reset",
	}); err != nil {
		t.Fatalf("handle reset callback: %v", err)
	}

	if repo.deletedRulesChatID != 99 {
		t.Fatalf("delete rules chat = %d, want 99", repo.deletedRulesChatID)
	}
	if !strings.Contains(replier.text, "Removed 17 saved alert filters") {
		t.Fatalf("reset text = %q, want removed count", replier.text)
	}
}

func TestFormatRulesSummaryGroupsCoverage(t *testing.T) {
	rules := []Rule{
		{ID: 1, Exchange: "okx", Coin: "usdt", Chain: "ethereum", EventTypes: AvailabilityEventTypes()},
		{ID: 2, Exchange: "okx", Coin: "usdt", Chain: "bsc", EventTypes: AvailabilityEventTypes()},
		{ID: 3, Exchange: "okx", Coin: "usdc", Chain: "base", EventTypes: AvailabilityEventTypes()},
	}

	summary := formatRulesSummary(rules)
	for _, want := range []string{
		"3 saved filters",
		"Events: route availability changes",
		"OKX",
		"- USDT: Ethereum, BSC",
		"- USDC: Base",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "deposit_off") {
		t.Fatalf("summary leaked raw event list:\n%s", summary)
	}
}

type fakeRepository struct {
	deleteResult       bool
	deleteRulesResult  int64
	deletedChatID      int64
	deletedRuleID      int64
	deletedRulesChatID int64
	created            []SubscribeCommand
}

func (f *fakeRepository) EnsureUser(ctx context.Context, chatID int64) (User, error) {
	return User{ID: 1, TGChatID: chatID}, nil
}

func (f *fakeRepository) CreateRule(ctx context.Context, chatID int64, cmd SubscribeCommand) (Rule, error) {
	f.created = append(f.created, cmd)
	return Rule{ID: 1, Exchange: cmd.Exchange, Coin: cmd.Coin, Chain: cmd.Chain, EventTypes: cmd.EventTypes}, nil
}

func (f *fakeRepository) ListRules(ctx context.Context, chatID int64) ([]Rule, error) {
	return nil, nil
}

func (f *fakeRepository) DeleteRule(ctx context.Context, chatID int64, ruleID int64) (bool, error) {
	f.deletedChatID = chatID
	f.deletedRuleID = ruleID
	return f.deleteResult, nil
}

func (f *fakeRepository) DeleteRules(ctx context.Context, chatID int64) (int64, error) {
	f.deletedRulesChatID = chatID
	return f.deleteRulesResult, nil
}

func (f *fakeRepository) FindRails(ctx context.Context, filter StatusCommand, limit int) ([]RailStatus, error) {
	return nil, nil
}

type fakeReplier struct {
	chatID            int64
	text              string
	keyboard          Keyboard
	edits             []fakeEdit
	answeredCallbacks []string
}

func (f *fakeReplier) Reply(ctx context.Context, chatID int64, text string) error {
	f.chatID = chatID
	f.text = text
	return nil
}

func (f *fakeReplier) ReplyWithKeyboard(ctx context.Context, chatID int64, text string, keyboard Keyboard) error {
	f.chatID = chatID
	f.text = text
	f.keyboard = keyboard
	return nil
}

func (f *fakeReplier) EditMessage(ctx context.Context, chatID int64, messageID int, text string, keyboard Keyboard) error {
	f.chatID = chatID
	f.text = text
	f.keyboard = keyboard
	f.edits = append(f.edits, fakeEdit{chatID: chatID, messageID: messageID, text: text, keyboard: keyboard})
	return nil
}

func (f *fakeReplier) AnswerCallback(ctx context.Context, callbackID string, text string) error {
	f.answeredCallbacks = append(f.answeredCallbacks, callbackID)
	return nil
}

type fakeEdit struct {
	chatID    int64
	messageID int
	text      string
	keyboard  Keyboard
}
