package commands

import "context"

type Button struct {
	Text string
	Data string
}

type Keyboard [][]Button

type KeyboardReplier interface {
	ReplyWithKeyboard(ctx context.Context, chatID int64, text string, keyboard Keyboard) error
}

type MessageEditor interface {
	EditMessage(ctx context.Context, chatID int64, messageID int, text string, keyboard Keyboard) error
}

type CallbackAcknowledger interface {
	AnswerCallback(ctx context.Context, callbackID string, text string) error
}
