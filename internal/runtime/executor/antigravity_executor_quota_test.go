package executor

import (
	"strings"
	"testing"
)

func TestDecodeAntigravityFiveHourQuotas(t *testing.T) {
	result, err := decodeAntigravityFiveHourQuotas(strings.NewReader(`{"groups":[{"buckets":[
		{"bucketId":"gemini-5h","window":"5h","remainingFraction":0,"resetTime":"2026-11-20T18:00:00Z"},
		{"bucketId":"3p-5h","window":"5h","remainingFraction":0.9,"resetTime":"2026-11-20T18:00:00Z"},
		{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.8,"resetTime":"2026-11-26T18:00:00Z"},
		{"bucketId":"gemini-argon-5h","window":"5h","resetTime":"2026-11-20T18:00:00Z"}
	]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result["gemini-5h"].ResetsAt.IsZero() || result["gemini-5h"].Remaining != 0 || result["3p-5h"].Remaining != .9 {
		t.Fatalf("incorrect five-hour buckets: %+v", result)
	}
}
