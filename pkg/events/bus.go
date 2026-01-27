package events

import (
	"sync"
)

type EventType string

const (
	ProjectParked       EventType = "ProjectParked"
	Projectforgotten    EventType = "ProjectForgotten"
	ServiceStarted      EventType = "ServiceStarted"
	ServiceStopped      EventType = "ServiceStopped"
	ConfigChanged       EventType = "ConfigChanged"
	SitesUpdated        EventType = "sites:updated"
	XRayLog             EventType = "xray:log"
	LogEntry            EventType = "log:entry"
	ArtisanOutput       EventType = "artisan:output"
	ArtisanDone         EventType = "artisan:done"
	HealerIssueDetected EventType = "healer:issue_detected"
	HealerIssueResolved EventType = "healer:issue_resolved"
)

type Event struct {
	Type    EventType
	Payload interface{}
}

type Handler func(Event)

type Bus struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
}

func NewBus() *Bus {
	return &Bus{
		handlers: make(map[EventType][]Handler),
	}
}

func (b *Bus) Subscribe(topic EventType, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], handler)
}

func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if handlers, ok := b.handlers[event.Type]; ok {
		for _, h := range handlers {
			// Run handlers synchronously for now to ensure consistency,
			// but could be goroutines in the future.
			h(event)
		}
	}
}
