package diff

import (
	"context"
	"errors"
	"fmt"
)

var ErrNilWriter = errors.New("diff writer is nil")

type Writer interface {
	UpsertRails(ctx context.Context, mutations []RailMutation) error
	InsertEvents(ctx context.Context, events []Event) error
}

func Apply(ctx context.Context, writer Writer, result Result) error {
	if writer == nil {
		return ErrNilWriter
	}
	if len(result.Rails) > 0 {
		if err := writer.UpsertRails(ctx, result.Rails); err != nil {
			return fmt.Errorf("upsert rails: %w", err)
		}
	}
	if len(result.Events) > 0 {
		if err := writer.InsertEvents(ctx, result.Events); err != nil {
			return fmt.Errorf("insert rail events: %w", err)
		}
	}
	return nil
}
