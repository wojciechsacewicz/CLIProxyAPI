package cliproxy

import (
	"context"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func (s *Service) runAntigravityFiveHourRefresh(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		s.refreshAntigravityFiveHourQuotas(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) refreshAntigravityFiveHourQuotas(ctx context.Context) {
	if s.coreManager == nil {
		return
	}
	selector := coreauth.UnwrapFiveHourSelector(s.coreManager.Selector())
	if selector == nil {
		return
	}
	executor, ok := s.coreManager.Executor("antigravity")
	if !ok {
		return
	}
	reader, ok := executor.(interface {
		FetchAntigravityFiveHourQuotas(context.Context, *coreauth.Auth) (map[string]coreauth.FiveHourWindow, error)
	})
	if !ok {
		return
	}

	var accounts []*coreauth.Auth
	for _, auth := range s.coreManager.List() {
		if auth.Provider == "antigravity" && !auth.Disabled && auth.Status != coreauth.StatusDisabled {
			accounts = append(accounts, auth)
		}
	}
	if len(accounts) < 2 {
		selector.ReplaceAntigravityQuotas(nil)
		return
	}
	snapshots := make(map[string]coreauth.AntigravityFiveHourQuota, len(accounts))
	for _, auth := range accounts {
		observed := time.Now()
		probe, cancel := context.WithTimeout(ctx, 10*time.Second)
		windows, err := reader.FetchAntigravityFiveHourQuotas(probe, auth)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil && len(windows) > 0 {
			snapshots[auth.ID] = coreauth.AntigravityFiveHourQuota{Epoch: auth.RegistrationEpoch, ObservedAt: observed, Windows: windows}
		}
	}
	selector.ReplaceAntigravityQuotas(snapshots)
}
