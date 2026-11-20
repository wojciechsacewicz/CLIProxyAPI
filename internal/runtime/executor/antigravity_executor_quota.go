package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/MoeForge/moego/json"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

type antigravityQuotaBucket struct {
	ID        string    `json:"bucketId"`
	Window    string    `json:"window"`
	Remaining *float64  `json:"remainingFraction"`
	Reset     time.Time `json:"resetTime"`
	Disabled  bool      `json:"disabled"`
}

// FetchAntigravityFiveHourQuotas reads quota metadata without sending an inference request.
func (e *AntigravityExecutor) FetchAntigravityFiveHourQuotas(ctx context.Context, auth *cliproxyauth.Auth) (map[string]cliproxyauth.FiveHourWindow, error) {
	project := antigravityProjectIDFromAuth(auth)
	if project == "" {
		return nil, fmt.Errorf("antigravity quota: missing project")
	}
	body, err := json.Marshal(map[string]string{"project": project})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveAntigravityRequestBaseURL(auth)+"/v1internal:retrieveUserQuotaSummary", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// The quota endpoint exposes Gemini buckets to current Hub clients.
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	req.Header.Set("User-Agent", fmt.Sprintf("antigravity/hub/2.24.0 (aidev_client; os_type=%s; arch=%s; cl=963137146)", runtime.GOOS, arch))
	if err := e.PrepareRequest(req, auth); err != nil {
		return nil, err
	}
	resp, err := newAntigravityHTTPClient(ctx, e.cfg, auth, 0).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("antigravity quota: HTTP %d", resp.StatusCode)
	}
	return decodeAntigravityFiveHourQuotas(io.LimitReader(resp.Body, 1<<20))
}

func decodeAntigravityFiveHourQuotas(body io.Reader) (map[string]cliproxyauth.FiveHourWindow, error) {
	var summary struct {
		Buckets []antigravityQuotaBucket `json:"buckets"`
		Groups  []struct {
			Buckets []antigravityQuotaBucket `json:"buckets"`
		} `json:"groups"`
	}
	if err := json.NewDecoder(body).Decode(&summary); err != nil {
		return nil, err
	}
	var grouped []antigravityQuotaBucket
	for _, group := range summary.Groups {
		grouped = append(grouped, group.Buckets...)
	}
	if len(grouped) > 0 {
		summary.Buckets = grouped
	}
	result := make(map[string]cliproxyauth.FiveHourWindow)
	for _, bucket := range summary.Buckets {
		if bucket.ID == "" || bucket.Disabled || bucket.Window != "5h" || bucket.Reset.IsZero() ||
			bucket.Remaining == nil || *bucket.Remaining < 0 || *bucket.Remaining > 1 {
			continue
		}
		result[bucket.ID] = cliproxyauth.FiveHourWindow{Remaining: *bucket.Remaining, ResetsAt: bucket.Reset}
	}
	return result, nil
}
