package auth

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

const codexFiveHourObservationMaxAge = 10 * time.Minute

// CodexFiveHourSelector ranks eligible Codex credentials by remaining five-hour quota.
// It does not perform network I/O and lets unobserved credentials collect data via round-robin.
type CodexFiveHourSelector struct {
	fallback RoundRobinSelector
}

func (s *CodexFiveHourSelector) Pick(ctx context.Context, provider, model string, _ cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	now := time.Now()
	available, err := getSelectorAvailableAuths(ctx, auths, provider, model, now)
	if err != nil {
		return nil, err
	}
	available = preferCodexWebsocketAuths(ctx, provider, available)
	if len(available) == 1 {
		return available[0], nil
	}

	var best []*Auth
	var bestRemaining float64
	for _, candidate := range available {
		remaining, ok := codexFiveHourRemaining(candidate, now)
		if !ok {
			return s.fallback.pickAvailable(provider, model, available), nil
		}
		if len(best) == 0 || remaining > bestRemaining {
			best = best[:0]
			bestRemaining = remaining
		}
		if remaining == bestRemaining {
			best = append(best, candidate)
		}
	}
	return s.fallback.pickAvailable(provider, model, best), nil
}

func codexFiveHourRemaining(auth *Auth, now time.Time) (float64, bool) {
	if !strings.EqualFold(auth.Provider, "codex") {
		return 0, false
	}
	quota := auth.Quota
	age := now.Sub(quota.ObservedAt)
	if quota.ObservedAt.IsZero() || age < 0 || age > codexFiveHourObservationMaxAge {
		return 0, false
	}

	for _, window := range [...]string{"X-Codex-Primary-", "X-Codex-Secondary-"} {
		if quota.Signals[http.CanonicalHeaderKey(window+"Window-Minutes")] != "300" {
			continue
		}
		reset, errReset := strconv.ParseInt(quota.Signals[http.CanonicalHeaderKey(window+"Reset-At")], 10, 64)
		used, errUsed := strconv.ParseFloat(quota.Signals[http.CanonicalHeaderKey(window+"Used-Percent")], 64)
		if errReset != nil || !time.Unix(reset, 0).After(now) || errUsed != nil || !(0 <= used && used <= 100) {
			continue
		}
		return 100 - used, true
	}
	return 0, false
}
