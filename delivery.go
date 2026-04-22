package scheduler

import (
	"context"
	"fmt"
	"strings"
)

// MessageSender sends a text message to a target address via some transport.
type MessageSender interface {
	Send(ctx context.Context, target, text string) error
	Name() string
}

// DeliveryService routes completed job output to the originating channel.
type DeliveryService interface {
	Deliver(ctx context.Context, result *JobResult) error
	Close() error
}

// RouterDelivery routes based on the channel-key prefix ("tg:123" → prefix "tg", target "123").
type RouterDelivery struct {
	senders map[string]MessageSender
	scanner func(string) string // credential redaction; may be nil
}

// NewRouterDelivery creates a RouterDelivery. senders maps prefix → sender.
// scanner is an optional credential-redaction function applied before sending.
func NewRouterDelivery(senders map[string]MessageSender, scanner func(string) string) *RouterDelivery {
	return &RouterDelivery{senders: senders, scanner: scanner}
}

// Deliver routes the result to the appropriate sender based on ChannelKey prefix.
// If result.Silent is true delivery is skipped. If result.Error is non-empty
// the error text is sent instead of the output.
func (r *RouterDelivery) Deliver(ctx context.Context, result *JobResult) error {
	if result.Silent {
		return nil
	}
	prefix, target, ok := splitChannelKey(result.ChannelKey)
	if !ok {
		return fmt.Errorf("delivery: malformed channel key %q (want prefix:target)", result.ChannelKey)
	}
	sender, ok := r.senders[prefix]
	if !ok {
		return fmt.Errorf("delivery: no sender registered for prefix %q", prefix)
	}
	text := result.Output
	if result.Error != "" {
		text = "error: " + result.Error
	}
	if r.scanner != nil {
		text = r.scanner(text)
	}
	return sender.Send(ctx, target, text)
}

// Close is a no-op for RouterDelivery.
func (r *RouterDelivery) Close() error { return nil }

// splitChannelKey parses "prefix:target" into its components.
func splitChannelKey(key string) (prefix, target string, ok bool) {
	idx := strings.Index(key, ":")
	if idx <= 0 {
		return "", "", false
	}
	return key[:idx], key[idx+1:], true
}
