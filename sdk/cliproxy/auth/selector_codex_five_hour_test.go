package auth

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func fiveHourAuth(id string, used int, now time.Time) *Auth {
	auth := &Auth{ID: id, Provider: "codex"}
	headers := make(http.Header)
	headers.Set("X-Codex-Primary-Used-Percent", strconv.Itoa(used))
	headers.Set("X-Codex-Primary-Window-Minutes", "300")
	headers.Set("X-Codex-Primary-Reset-At", strconv.FormatInt(now.Add(time.Hour).Unix(), 10))
	auth.Quota.ObserveResponseHeadersForProvider("codex", headers, now)
	return auth
}

func TestCodexFiveHourRoutingKeepsExistingSession(t *testing.T) {
	now := time.Now()
	a, b := fiveHourAuth("a", 80, now), fiveHourAuth("b", 10, now)
	pool := []*Auth{a, b}
	selector := NewSessionAffinitySelector(&CodexFiveHourSelector{})
	t.Cleanup(selector.Stop)
	pick := func(session, want string) {
		t.Helper()
		opts := cliproxyexecutor.Options{Headers: http.Header{"Session_id": {session}}}
		got, err := selector.Pick(t.Context(), "mixed", "gpt-6.2-sol", opts, pool)
		if err != nil || got.ID != want {
			t.Fatalf("session %s: wanted %s, got %v (%v)", session, want, got, err)
		}
	}
	pick("new-best-1", "b")
	pick("new-best-2", "b")
	*a = *fiveHourAuth("a", 5, now)
	*b = *fiveHourAuth("b", 95, now)
	pick("new-best-1", "b")
	pick("after-quota-change", "a")
}

func TestCodexFiveHourRoutingFallsBackWhenQuotaIsUnknown(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name   string
		change func(*Auth)
	}{
		{"weekly only", func(a *Auth) { a.Quota.Signals["X-Codex-Primary-Window-Minutes"] = "10080" }},
		{"stale", func(a *Auth) { a.Quota.ObservedAt = now.Add(-11 * time.Minute) }},
		{"reset", func(a *Auth) { a.Quota.Signals["X-Codex-Primary-Reset-At"] = strconv.FormatInt(now.Unix(), 10) }},
		{"invalid", func(a *Auth) { a.Quota.Signals["X-Codex-Primary-Used-Percent"] = "NaN" }},
		{"different provider", func(a *Auth) { a.Provider = "claude" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := fiveHourAuth("a", 80, now), fiveHourAuth("b", 10, now)
			tc.change(a)
			selector := &CodexFiveHourSelector{}
			for _, want := range []string{"a", "b", "a"} {
				got, err := selector.Pick(t.Context(), "mixed", "gpt-6.2-sol", cliproxyexecutor.Options{}, []*Auth{a, b})
				if err != nil || got.ID != want {
					t.Fatalf("wanted %s, got %v (%v)", want, got, err)
				}
			}
		})
	}
}

func TestCodexFiveHourRoutingRespectsCooldownAndPriority(t *testing.T) {
	now := time.Now()
	a, b := fiveHourAuth("a", 80, now), fiveHourAuth("b", 10, now)
	selector := &CodexFiveHourSelector{}
	pick := func(want string) {
		t.Helper()
		got, err := selector.Pick(t.Context(), "codex", "gpt-6.2-sol", cliproxyexecutor.Options{}, []*Auth{a, b})
		if err != nil || got.ID != want {
			t.Fatalf("wanted %s, got %v (%v)", want, got, err)
		}
	}
	a.Attributes = map[string]string{"priority": "1"}
	pick("a")
	a.Attributes = nil
	b.Disabled = true
	pick("a")
	b.Disabled = false
	b.ModelStates = map[string]*ModelState{"gpt-6.2-sol": {Unavailable: true, NextRetryAfter: now.Add(time.Minute)}}
	pick("a")
}

func TestCodexFiveHourRemainingReadsSecondaryWindow(t *testing.T) {
	now := time.Now()
	auth := fiveHourAuth("a", 20, now)
	headers := make(http.Header)
	headers.Set("X-Codex-Primary-Window-Minutes", "10080")
	headers.Set("X-Codex-Primary-Used-Percent", "8")
	headers.Set("X-Codex-Secondary-Window-Minutes", "300")
	headers.Set("X-Codex-Secondary-Used-Percent", "20")
	headers.Set("X-Codex-Secondary-Reset-At", strconv.FormatInt(now.Add(time.Hour).Unix(), 10))
	auth.Quota.ObserveResponseHeadersForProvider("codex", headers, now)
	got, ok := codexFiveHourRemaining(auth, now)
	if !ok || got != 80 {
		t.Fatalf("want 80 percent left, got %v (valid=%t)", got, ok)
	}
}
