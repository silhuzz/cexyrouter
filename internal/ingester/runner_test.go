package ingester

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pedro/cex-router/pkg/types"
)

func TestRunOnceHonorsMaxConcurrencyAndKeepsInputOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	adapters := []types.Adapter{
		fakeAdapter{slug: "one"},
		fakeAdapter{slug: "two"},
		fakeAdapter{slug: "three"},
		fakeAdapter{slug: "four"},
	}

	var mu sync.Mutex
	current := 0
	maxSeen := 0
	runner := &Runner{
		MaxConcurrency: 2,
		runAdapter: func(ctx context.Context, adapter types.Adapter) (CycleResult, error) {
			mu.Lock()
			current++
			if current > maxSeen {
				maxSeen = current
			}
			mu.Unlock()

			select {
			case <-time.After(25 * time.Millisecond):
			case <-ctx.Done():
				return CycleResult{}, ctx.Err()
			}

			mu.Lock()
			current--
			mu.Unlock()

			return CycleResult{ExchangeSlug: adapter.Slug()}, nil
		},
	}

	results, err := runner.RunOnce(ctx, adapters)
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if maxSeen != 2 {
		t.Fatalf("max concurrency = %d, want 2", maxSeen)
	}
	if len(results) != len(adapters) {
		t.Fatalf("results len = %d, want %d", len(results), len(adapters))
	}
	for i, result := range results {
		if result.ExchangeSlug != adapters[i].Slug() {
			t.Fatalf("result %d exchange = %q, want %q", i, result.ExchangeSlug, adapters[i].Slug())
		}
	}
}

func TestRunOnceIsolatesAdapterErrors(t *testing.T) {
	ctx := context.Background()
	adapters := []types.Adapter{
		fakeAdapter{slug: "ok-a"},
		fakeAdapter{slug: "bad"},
		fakeAdapter{slug: "ok-b"},
	}
	failErr := errors.New("adapter failed")
	runner := &Runner{
		MaxConcurrency: 3,
		runAdapter: func(ctx context.Context, adapter types.Adapter) (CycleResult, error) {
			if adapter.Slug() == "bad" {
				return CycleResult{}, failErr
			}
			return CycleResult{ExchangeSlug: adapter.Slug()}, nil
		},
	}

	results, err := runner.RunOnce(ctx, adapters)
	if !errors.Is(err, failErr) {
		t.Fatalf("RunOnce error = %v, want wrapped adapter error", err)
	}
	if results[0].Error != nil || results[2].Error != nil {
		t.Fatalf("healthy adapters were marked failed: %+v", results)
	}
	if !errors.Is(results[1].Error, failErr) {
		t.Fatalf("failed adapter result error = %v, want %v", results[1].Error, failErr)
	}
	if results[1].ExchangeSlug != "bad" {
		t.Fatalf("failed adapter exchange = %q, want bad", results[1].ExchangeSlug)
	}
}

type fakeAdapter struct {
	slug string
}

func (f fakeAdapter) Slug() string {
	return f.slug
}

func (f fakeAdapter) FetchRails(context.Context) (types.FetchResult, error) {
	return types.FetchResult{}, nil
}
