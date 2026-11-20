package auth

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

const fiveHourObservationMaxAge = 10 * time.Minute

// FiveHourWindow is a five-hour quota observation for an Antigravity model family.
type FiveHourWindow struct {
	Remaining float64 // Fraction from zero to one.
	ResetsAt  time.Time
}

// AntigravityFiveHourQuota is an immutable quota snapshot for one auth registration.
type AntigravityFiveHourQuota struct {
	Epoch      uint64
	ObservedAt time.Time
	Windows    map[string]FiveHourWindow
}

// FiveHourSelector ranks eligible accounts of the same provider by remaining five-hour quota.
type FiveHourSelector struct {
	fallback    RoundRobinSelector
	antigravity atomic.Pointer[map[string]AntigravityFiveHourQuota]
}

func UnwrapFiveHourSelector(selector Selector) *FiveHourSelector {
	if affinity, ok := selector.(*SessionAffinitySelector); ok {
		selector = affinity.fallback
	}
	value, _ := selector.(*FiveHourSelector)
	return value
}

// ReplaceAntigravityQuotas publishes a snapshot. Callers must not modify it after publication.
func (s *FiveHourSelector) ReplaceAntigravityQuotas(quotas map[string]AntigravityFiveHourQuota) {
	s.antigravity.Store(&quotas)
}

func (s *FiveHourSelector) Pick(ctx context.Context, provider, model string, _ cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
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
	snapshot := s.antigravity.Load()
	for _, candidate := range available {
		if !strings.EqualFold(candidate.Provider, available[0].Provider) {
			return s.fallback.pickAvailable(provider, model, available), nil
		}
		remaining, ok := fiveHourRemaining(candidate, model, now, snapshot)
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

func freshFiveHourObservation(observedAt, now time.Time) bool {
	age := now.Sub(observedAt)
	return !observedAt.IsZero() && age >= 0 && age <= fiveHourObservationMaxAge
}

func fiveHourRemaining(auth *Auth, model string, now time.Time, snapshot *map[string]AntigravityFiveHourQuota) (float64, bool) {
	switch strings.ToLower(auth.Provider) {
	case "antigravity":
		if snapshot == nil {
			return 0, false
		}
		quota := (*snapshot)[auth.ID]
		window, ok := quota.Windows[antigravityFiveHourBucket(model)]
		if !ok || quota.Epoch != auth.RegistrationEpoch || !freshFiveHourObservation(quota.ObservedAt, now) || !window.ResetsAt.After(now) {
			return 0, false
		}
		return window.Remaining * 100, true
	case "codex":
		return codexFiveHourRemaining(auth, now)
	case "claude":
		if !freshFiveHourObservation(auth.Quota.ObservedAt, now) {
			return 0, false
		}
		return fiveHourFromUsed(auth.Quota.Signals, "Anthropic-Ratelimit-Unified-5h-Utilization", "Anthropic-Ratelimit-Unified-5h-Reset", 1, now)
	default:
		return 0, false
	}
}

func codexFiveHourRemaining(auth *Auth, now time.Time) (float64, bool) {
	if !freshFiveHourObservation(auth.Quota.ObservedAt, now) {
		return 0, false
	}
	for _, window := range [...]string{"X-Codex-Primary-", "X-Codex-Secondary-"} {
		if auth.Quota.Signals[window+"Window-Minutes"] == "300" {
			if remaining, ok := fiveHourFromUsed(auth.Quota.Signals, window+"Used-Percent", window+"Reset-At", 100, now); ok {
				return remaining, true
			}
		}
	}
	return 0, false
}

func fiveHourFromUsed(signals map[string]string, usedKey, resetKey string, scale float64, now time.Time) (float64, bool) {
	reset, errReset := strconv.ParseInt(signals[resetKey], 10, 64)
	used, errUsed := strconv.ParseFloat(signals[usedKey], 64)
	if errReset != nil || !time.Unix(reset, 0).After(now) || errUsed != nil || !(0 <= used && used <= scale) {
		return 0, false
	}
	return (1 - used/scale) * 100, true
}

func antigravityFiveHourBucket(model string) string {
	model = strings.ToLower(canonicalModelKey(model))
	if index := strings.LastIndexByte(model, '/'); index >= 0 {
		model = model[index+1:]
	}
	switch {
	case strings.HasPrefix(model, "gemini-4-argon"):
		return "gemini-argon-5h"
	case strings.HasPrefix(model, "gemini-4-carbon"):
		return "gemini-carbon-5h"
	case strings.HasPrefix(model, "gemini-"):
		return "gemini-5h"
	case strings.HasPrefix(model, "claude-"), strings.HasPrefix(model, "gpt-oss-"):
		return "3p-5h"
	default:
		return ""
	}
}
