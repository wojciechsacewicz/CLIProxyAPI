package auth

import (
	"context"
	"path/filepath"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
)

type FiveHourPreview struct {
	Reason    string   `json:"reason"`
	Account   string   `json:"account,omitempty"`
	Provider  string   `json:"provider,omitempty"`
	Remaining *float64 `json:"remaining_percent,omitempty"`
}

// PreviewFiveHour predicts an unpinned request without invoking a scheduler, advancing its cursor,
// binding a session, or refreshing credentials. Ties and fallbacks have no predicted account.
func (m *Manager) PreviewFiveHour(ctx context.Context, providers []string, model string) FiveHourPreview {
	if m == nil || m.HomeEnabled() || m.hasPluginScheduler() {
		return FiveHourPreview{Reason: "unsupported"}
	}
	providerSet := make(map[string]bool, len(providers))
	for _, provider := range providers {
		providerSet[canonicalSchedulingProvider(provider)] = true
	}
	reg := registry.GetGlobalRegistry()
	now := time.Now()
	m.mu.RLock()
	selector := UnwrapFiveHourSelector(m.selector)
	if selector == nil {
		m.mu.RUnlock()
		return FiveHourPreview{Reason: "unsupported"}
	}
	candidates := make([]*Auth, 0, len(m.auths))
	for _, candidate := range m.auths {
		if candidate == nil || candidate.Disabled {
			continue
		}
		provider := executorKeyFromAuth(candidate)
		if !providerSet[provider] || !m.authSupportsRouteModel(reg, candidate, model) {
			continue
		}
		if _, ok := m.executorLocked(provider); ok {
			candidates = append(candidates, candidate)
		}
	}
	available, _, err := m.availableAuthsForSelector(ctx, selector, candidates, "mixed", model, now, false)
	m.mu.RUnlock()
	if err != nil || len(available) == 0 {
		return FiveHourPreview{Reason: "unavailable"}
	}
	best := available
	var remaining *float64
	reason := "only-account"
	if len(available) > 1 {
		var percent float64
		best, percent, reason = selector.rankAvailable(model, now, available)
		if reason != "quota" {
			return FiveHourPreview{Reason: reason}
		}
		if len(best) != 1 {
			return FiveHourPreview{Reason: "tie"}
		}
		remaining = &percent
	}
	chosen := best[0]
	account := chosen.EnsureIndex()
	if chosen.FileName != "" {
		account = filepath.Base(chosen.FileName)
	}
	return FiveHourPreview{Reason: reason, Account: account, Provider: chosen.Provider, Remaining: remaining}
}
