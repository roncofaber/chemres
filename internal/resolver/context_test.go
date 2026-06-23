package resolver

import (
	"context"
	"testing"
)

func TestWithClientIP_RoundTrip(t *testing.T) {
	ctx := WithClientIP(context.Background(), "1.2.3.4")
	if got := clientIPFromCtx(ctx); got != "1.2.3.4" {
		t.Errorf("got %q, want %q", got, "1.2.3.4")
	}
}

func TestClientIPFromCtx_Missing(t *testing.T) {
	if got := clientIPFromCtx(context.Background()); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}
