package scheduler

import (
	"context"
	"strings"
	"testing"
)

// fakeSender records Send calls for assertions.
type fakeSender struct {
	target string
	text   string
}

func (f *fakeSender) Send(_ context.Context, target, text string) error {
	f.target = target
	f.text = text
	return nil
}
func (f *fakeSender) Name() string { return "fake" }

func TestRouterDelivery_RouteToSender(t *testing.T) {
	t.Parallel()
	tg := &fakeSender{}
	r := NewRouterDelivery(map[string]MessageSender{"tg": tg}, nil)
	err := r.Deliver(t.Context(), &JobResult{
		ChannelKey: "tg:42",
		Output:     "job output",
		Status:     StatusSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tg.target != "42" {
		t.Fatalf("target = %q; want 42", tg.target)
	}
	if tg.text != "job output" {
		t.Fatalf("text = %q; want 'job output'", tg.text)
	}
}

func TestRouterDelivery_Silent_Skips(t *testing.T) {
	t.Parallel()
	tg := &fakeSender{}
	r := NewRouterDelivery(map[string]MessageSender{"tg": tg}, nil)
	_ = r.Deliver(t.Context(), &JobResult{
		ChannelKey: "tg:42", Output: "x", Silent: true,
	})
	if tg.text != "" {
		t.Fatal("silent should skip delivery")
	}
}

func TestRouterDelivery_UnknownPrefix_Error(t *testing.T) {
	t.Parallel()
	r := NewRouterDelivery(map[string]MessageSender{}, nil)
	err := r.Deliver(t.Context(), &JobResult{ChannelKey: "tg:1", Output: "x"})
	if err == nil {
		t.Fatal("expected error for unknown prefix")
	}
}

func TestRouterDelivery_MalformedKey_Error(t *testing.T) {
	t.Parallel()
	r := NewRouterDelivery(map[string]MessageSender{}, nil)
	err := r.Deliver(t.Context(), &JobResult{ChannelKey: "nocoion", Output: "x"})
	if err == nil {
		t.Fatal("expected error for malformed key")
	}
}

func TestRouterDelivery_RedactsSecrets(t *testing.T) {
	t.Parallel()
	tg := &fakeSender{}
	scanner := func(s string) string { return strings.ReplaceAll(s, "secret", "[REDACTED]") }
	r := NewRouterDelivery(map[string]MessageSender{"tg": tg}, scanner)
	_ = r.Deliver(t.Context(), &JobResult{ChannelKey: "tg:1", Output: "contains secret here"})
	if strings.Contains(tg.text, "secret") {
		t.Fatalf("secret not redacted: %q", tg.text)
	}
}

func TestRouterDelivery_ErrorResult_SendsErrorText(t *testing.T) {
	t.Parallel()
	tg := &fakeSender{}
	r := NewRouterDelivery(map[string]MessageSender{"tg": tg}, nil)
	_ = r.Deliver(t.Context(), &JobResult{
		ChannelKey: "tg:1",
		Output:     "",
		Error:      "job failed",
		Status:     StatusFailed,
	})
	if !strings.Contains(tg.text, "job failed") {
		t.Fatalf("error not propagated: %q", tg.text)
	}
}

func TestRouterDelivery_Close(t *testing.T) {
	t.Parallel()
	r := NewRouterDelivery(map[string]MessageSender{}, nil)
	if err := r.Close(); err != nil {
		t.Fatalf("Close should return nil, got: %v", err)
	}
}

func TestSplitChannelKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key    string
		prefix string
		target string
		ok     bool
	}{
		{"tg:123", "tg", "123", true},
		{"tg:", "tg", "", true},
		{":123", "", "", false},
		{"nocolon", "", "", false},
		{"", "", "", false},
		{"a:b:c", "a", "b:c", true},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			prefix, target, ok := splitChannelKey(tt.key)
			if ok != tt.ok || prefix != tt.prefix || target != tt.target {
				t.Errorf("splitChannelKey(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.key, prefix, target, ok, tt.prefix, tt.target, tt.ok)
			}
		})
	}
}
