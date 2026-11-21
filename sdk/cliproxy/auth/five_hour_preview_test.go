package auth

import (
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func TestPreviewFiveHourTracksAvailableQuotaWithoutMovingCursor(t *testing.T) {
	now := time.Now()
	selector := NewSessionAffinitySelector(&FiveHourSelector{})
	t.Cleanup(selector.Stop)
	manager := NewManager(nil, selector, nil)
	manager.RegisterExecutor(schedulerTestExecutor{provider: "codex"})
	model := "preview-five-hour-test"
	a, b := fiveHourAuth("preview-test-a", 80, now), fiveHourAuth("preview-test-b", 10, now)
	for _, account := range []*Auth{a, b} {
		account.FileName = account.ID + ".json"
		if _, err := manager.Register(WithSkipPersist(t.Context()), account); err != nil {
			t.Fatal(err)
		}
		registry.GetGlobalRegistry().RegisterClient(account.ID, "codex", []*registry.ModelInfo{{ID: model}})
		t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(account.ID) })
	}

	preview := func() FiveHourPreview {
		return manager.PreviewFiveHour(t.Context(), []string{"codex"}, model)
	}
	if got := preview(); got.Account != b.FileName || got.Remaining == nil || *got.Remaining != 90 {
		t.Fatalf("expected b with 90%% left, got %+v", got)
	}

	update := func(account *Auth) {
		t.Helper()
		if _, err := manager.Update(WithSkipPersist(t.Context()), account); err != nil {
			t.Fatal(err)
		}
	}
	b.Quota = a.Quota
	update(b)
	for range 3 {
		if got := preview(); got.Reason != "tie" || got.Account != "" {
			t.Fatalf("expected a tie with no prediction, got %+v", got)
		}
	}
	got, _, _, err := manager.pickNextMixed(t.Context(), []string{"codex"}, model, cliproxyexecutor.Options{}, nil)
	if err != nil || got.ID != a.ID {
		t.Fatalf("preview advanced the round-robin cursor: %v (%v)", got, err)
	}

	b.Quota.ObservedAt = now.Add(-time.Hour)
	update(b)
	if got := preview(); got.Reason != "quota" || got.Account != a.FileName {
		t.Fatalf("expected an account with fresh quota to rank ahead of stale quota, got %+v", got)
	}

	b.Quota = fiveHourAuth(b.ID, 25, now).Quota
	b.Quota.Signals["X-Codex-Primary-Window-Minutes"] = "10080"
	update(b)
	if got := preview(); got.Account != b.FileName || got.Remaining != nil || got.RankingPercent == nil || *got.RankingPercent != 75 {
		t.Fatalf("expected weekly-only b with a 75%% ranking score and no five-hour quota, got %+v", got)
	}

	b.Disabled = true
	update(b)
	if got := preview(); got.Reason != "only-account" || got.Account != a.FileName {
		t.Fatalf("expected a to be the only available account, got %+v", got)
	}
}
