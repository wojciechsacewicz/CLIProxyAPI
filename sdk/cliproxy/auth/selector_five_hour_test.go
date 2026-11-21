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
	selector := NewSessionAffinitySelector(&FiveHourSelector{})
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

func TestCodexFiveHourRoutingWithMissingQuota(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name   string
		change func(*Auth, *Auth)
		want   []string
	}{
		{"weekly only beats constrained account", func(a, b *Auth) {
			a.Quota.Signals["X-Codex-Primary-Window-Minutes"] = "10080"
			b.Quota.Signals["X-Codex-Secondary-Window-Minutes"] = "10080"
			b.Quota.Signals["X-Codex-Secondary-Used-Percent"] = "95"
			b.Quota.Signals["X-Codex-Secondary-Reset-At"] = strconv.FormatInt(now.Add(time.Hour).Unix(), 10)
		}, []string{"a", "a"}},
		{"stale", func(a, _ *Auth) { a.Quota.ObservedAt = now.Add(-11 * time.Minute) }, []string{"b", "b"}},
		{"reset", func(a, _ *Auth) { a.Quota.Signals["X-Codex-Primary-Reset-At"] = strconv.FormatInt(now.Unix(), 10) }, []string{"b", "b"}},
		{"invalid", func(a, _ *Auth) { a.Quota.Signals["X-Codex-Primary-Used-Percent"] = "NaN" }, []string{"b", "b"}},
		{"all unknown", func(a, b *Auth) { a.Quota, b.Quota = QuotaState{}, QuotaState{} }, []string{"a", "b", "a"}},
		{"different providers", func(a, _ *Auth) { a.Provider = "claude" }, []string{"a", "b", "a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := fiveHourAuth("a", 80, now), fiveHourAuth("b", 10, now)
			tc.change(a, b)
			selector := &FiveHourSelector{}
			for _, want := range tc.want {
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
	selector := &FiveHourSelector{}
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

func TestFiveHourRoutingRanksClaudeUtilization(t *testing.T) {
	now := time.Now()
	candidate := func(id, shortUsed, weeklyUsed string) *Auth {
		a := &Auth{ID: id, Provider: "claude"}
		h := make(http.Header)
		h.Set("Anthropic-Ratelimit-Unified-5h-Utilization", shortUsed)
		h.Set("Anthropic-Ratelimit-Unified-5h-Reset", strconv.FormatInt(now.Add(time.Hour).Unix(), 10))
		h.Set("Anthropic-Ratelimit-Unified-7d-Utilization", weeklyUsed)
		h.Set("Anthropic-Ratelimit-Unified-7d-Reset", strconv.FormatInt(now.AddDate(0, 0, 6).Unix(), 10))
		a.Quota.ObserveResponseHeadersForProvider("claude", h, now)
		return a
	}
	pool := []*Auth{
		candidate("a", "0.125", "0.875"),
		candidate("b", "", "0.5"),
		candidate("c", "0.75", "0.125"),
		{ID: "d", Provider: "claude"},
	}
	selector := &FiveHourSelector{}
	for range 2 {
		got, err := selector.Pick(t.Context(), "mixed", "claude-opus-5-5", cliproxyexecutor.Options{}, pool)
		if err != nil || got.ID != "b" {
			t.Fatalf("expected weekly-only b with the best remaining quota, got %v (%v)", got, err)
		}
	}
}

func TestFiveHourRoutingUsesAntigravityFamilyAndKeepsBindings(t *testing.T) {
	now := time.Now()
	reset := now.Add(time.Hour)
	a := &Auth{ID: "a", Provider: "antigravity", RegistrationEpoch: 1}
	b := &Auth{ID: "b", Provider: "antigravity", RegistrationEpoch: 1}
	pool := []*Auth{a, b}
	fallback := &FiveHourSelector{}
	selector := NewSessionAffinitySelector(fallback)
	t.Cleanup(selector.Stop)
	publish := func(neonA, neonB float64) {
		fallback.ReplaceAntigravityQuotas(map[string]AntigravityFiveHourQuota{
			"a": {Epoch: 1, ObservedAt: now, Windows: map[string]FiveHourWindow{"gemini-5h": {neonA, reset}, "3p-5h": {.9, reset}}},
			"b": {Epoch: 1, ObservedAt: now, Windows: map[string]FiveHourWindow{"gemini-5h": {neonB, reset}, "3p-5h": {.2, reset}}},
		})
	}
	pick := func(session, model, want string) {
		t.Helper()
		opts := cliproxyexecutor.Options{Headers: http.Header{"Session_id": {session}}}
		got, err := selector.Pick(t.Context(), "antigravity", model, opts, pool)
		if err != nil || got.ID != want {
			t.Fatalf("session %s: wanted %s, got %v (%v)", session, want, got, err)
		}
	}
	publish(.2, .9)
	pick("old-neon", "gemini-4-neon-xhigh", "b")
	pick("new-opus", "claude-opus-5-5-high", "a")
	publish(.8, .1)
	pick("old-neon", "gemini-4-neon-xhigh", "b")
	pick("new-neon", "gemini-4-neon-xhigh", "a")
	pick("unknown-argon-a", "gemini-4-argon-high", "a")
	pick("unknown-argon-b", "gemini-4-argon-high", "b")
	// A replaced login must not use the preceding registration's quota.
	b.RegistrationEpoch++
	_, ok := fiveHourRemaining(b, "gemini-4-neon-xhigh", now, fallback.antigravity.Load())
	if ok {
		t.Fatal("accepted quota from a prior auth registration")
	}
}
