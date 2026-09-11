package notify

import (
	"context"
	"sync"
)

// DeliveryMessage is the normalized payload passed to channel senders.
type DeliveryMessage struct {
	To      string
	Subject string
	Body    string
	Text    string
}

// Sender delivers notifications on a single channel.
type Sender interface {
	Channel() string
	Send(ctx context.Context, msg DeliveryMessage) error
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Sender{}
)

// Register adds or replaces a channel sender in the global registry.
func Register(s Sender) {
	if s == nil {
		return
	}
	registryMu.Lock()
	registry[s.Channel()] = s
	registryMu.Unlock()
}

// Get returns the sender for a channel, or nil if none is registered.
func Get(channel string) Sender {
	registryMu.RLock()
	s := registry[channel]
	registryMu.RUnlock()
	return s
}

// GetAll returns all registered senders.
func GetAll() []Sender {
	registryMu.RLock()
	out := make([]Sender, 0, len(registry))
	for _, s := range registry {
		out = append(out, s)
	}
	registryMu.RUnlock()
	return out
}
