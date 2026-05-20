package tg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pedro/cex-router/internal/tg/commands"
)

const telegramAPIEndpoint = "https://api.telegram.org"

type Poller interface {
	Updates(ctx context.Context) (<-chan commands.Message, error)
}

type Bot struct {
	poller  Poller
	handler *commands.Handler
	logger  *slog.Logger
}

func NewBot(poller Poller, repo commands.Repository, replier commands.Replier, logger *slog.Logger) *Bot {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bot{
		poller:  poller,
		handler: commands.NewHandler(repo, replier),
		logger:  logger,
	}
}

func (b *Bot) Run(ctx context.Context) error {
	if b == nil || b.poller == nil {
		return errors.New("telegram poller is not configured")
	}
	updates, err := b.poller.Updates(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-updates:
			if !ok {
				return nil
			}
			if err := b.handler.Handle(ctx, msg); err != nil {
				b.logger.Error("handle telegram command", "chat_id", msg.ChatID, "error", err)
			}
		}
	}
}

type TelegramClient struct {
	token       string
	endpoint    string
	httpClient  *http.Client
	pollTimeout time.Duration
	logger      *slog.Logger
}

func NewTelegramClient(token string, httpClient *http.Client, logger *slog.Logger) *TelegramClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 35 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TelegramClient{
		token:       strings.TrimSpace(token),
		endpoint:    telegramAPIEndpoint,
		httpClient:  httpClient,
		pollTimeout: 30 * time.Second,
		logger:      logger,
	}
}

func (c *TelegramClient) Updates(ctx context.Context) (<-chan commands.Message, error) {
	if c == nil || c.token == "" {
		return nil, errors.New("telegram token is not configured")
	}

	out := make(chan commands.Message)
	go c.pollUpdates(ctx, out)
	return out, nil
}

func (c *TelegramClient) Reply(ctx context.Context, chatID int64, text string) error {
	if c == nil || c.token == "" {
		return errors.New("telegram token is not configured")
	}
	var result telegramMessage
	return c.call(ctx, "sendMessage", map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}, &result)
}

func (c *TelegramClient) ReplyWithKeyboard(ctx context.Context, chatID int64, text string, keyboard commands.Keyboard) error {
	if c == nil || c.token == "" {
		return errors.New("telegram token is not configured")
	}
	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if len(keyboard) > 0 {
		payload["reply_markup"] = telegramReplyMarkup(keyboard)
	}
	var result telegramMessage
	return c.call(ctx, "sendMessage", payload, &result)
}

func (c *TelegramClient) EditMessage(ctx context.Context, chatID int64, messageID int, text string, keyboard commands.Keyboard) error {
	if c == nil || c.token == "" {
		return errors.New("telegram token is not configured")
	}
	if messageID <= 0 {
		return errors.New("telegram message id is not configured")
	}
	payload := map[string]any{
		"chat_id":                  chatID,
		"message_id":               messageID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if len(keyboard) > 0 {
		payload["reply_markup"] = telegramReplyMarkup(keyboard)
	}
	var result telegramMessage
	return c.call(ctx, "editMessageText", payload, &result)
}

func (c *TelegramClient) AnswerCallback(ctx context.Context, callbackID string, text string) error {
	if c == nil || c.token == "" {
		return errors.New("telegram token is not configured")
	}
	callbackID = strings.TrimSpace(callbackID)
	if callbackID == "" {
		return nil
	}
	payload := map[string]any{
		"callback_query_id": callbackID,
	}
	if strings.TrimSpace(text) != "" {
		payload["text"] = text
	}
	return c.call(ctx, "answerCallbackQuery", payload, nil)
}

func (c *TelegramClient) pollUpdates(ctx context.Context, out chan<- commands.Message) {
	defer close(out)

	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return
		}

		updates, err := c.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.logger.Warn("telegram long poll failed", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				continue
			}
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			msg, ok := update.toCommandMessage()
			if !ok {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case out <- msg:
			}
		}
	}
}

func (c *TelegramClient) getUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	payload := map[string]any{
		"timeout":         int(c.pollTimeout / time.Second),
		"allowed_updates": []string{"message", "callback_query"},
	}
	if offset > 0 {
		payload["offset"] = offset
	}

	var updates []telegramUpdate
	if err := c.call(ctx, "getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *TelegramClient) call(ctx context.Context, method string, payload any, result any) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return fmt.Errorf("encode telegram request: %w", err)
	}

	endpoint := strings.TrimRight(c.endpoint, "/") + "/bot" + c.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call telegram %s: %w", method, err)
	}
	defer resp.Body.Close()

	var apiResp telegramResponse[json.RawMessage]
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("decode telegram %s response: %w", method, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !apiResp.OK {
		if apiResp.Description == "" {
			apiResp.Description = resp.Status
		}
		return fmt.Errorf("telegram %s failed: %s", method, apiResp.Description)
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(apiResp.Result, result); err != nil {
		return fmt.Errorf("decode telegram %s result: %w", method, err)
	}
	return nil
}

type telegramResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

type telegramUpdate struct {
	UpdateID      int64                  `json:"update_id"`
	Message       *telegramMessage       `json:"message"`
	CallbackQuery *telegramCallbackQuery `json:"callback_query"`
}

type telegramMessage struct {
	MessageID int          `json:"message_id"`
	Chat      telegramChat `json:"chat"`
	Text      string       `json:"text"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramCallbackQuery struct {
	ID      string           `json:"id"`
	Message *telegramMessage `json:"message"`
	Data    string           `json:"data"`
}

type telegramInlineKeyboardMarkup struct {
	InlineKeyboard [][]telegramInlineKeyboardButton `json:"inline_keyboard"`
}

type telegramInlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

func (u telegramUpdate) toCommandMessage() (commands.Message, bool) {
	if u.CallbackQuery != nil {
		if u.CallbackQuery.Message == nil || strings.TrimSpace(u.CallbackQuery.Data) == "" {
			return commands.Message{}, false
		}
		return commands.Message{
			ChatID:          u.CallbackQuery.Message.Chat.ID,
			MessageID:       u.CallbackQuery.Message.MessageID,
			CallbackQueryID: u.CallbackQuery.ID,
			CallbackData:    u.CallbackQuery.Data,
		}, true
	}
	if u.Message == nil || strings.TrimSpace(u.Message.Text) == "" {
		return commands.Message{}, false
	}
	return commands.Message{
		ChatID:    u.Message.Chat.ID,
		MessageID: u.Message.MessageID,
		Text:      u.Message.Text,
	}, true
}

func telegramReplyMarkup(keyboard commands.Keyboard) telegramInlineKeyboardMarkup {
	rows := make([][]telegramInlineKeyboardButton, 0, len(keyboard))
	for _, row := range keyboard {
		buttons := make([]telegramInlineKeyboardButton, 0, len(row))
		for _, button := range row {
			buttons = append(buttons, telegramInlineKeyboardButton{
				Text:         button.Text,
				CallbackData: button.Data,
			})
		}
		if len(buttons) > 0 {
			rows = append(rows, buttons)
		}
	}
	return telegramInlineKeyboardMarkup{InlineKeyboard: rows}
}

var _ Poller = (*TelegramClient)(nil)
var _ commands.Replier = (*TelegramClient)(nil)
var _ commands.KeyboardReplier = (*TelegramClient)(nil)
var _ commands.MessageEditor = (*TelegramClient)(nil)
var _ commands.CallbackAcknowledger = (*TelegramClient)(nil)
