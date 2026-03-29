package patterns

// Observer receives notifications.
type Observer interface {
	OnNotify(event string)
}

// EventBus manages subscriptions and notifications.
type EventBus struct {
	Listeners []Observer
}

// Subscribe registers an observer.
func (b *EventBus) Subscribe(o Observer) {
	b.Listeners = append(b.Listeners, o)
}

// Notify sends an event to all observers.
func (b *EventBus) Notify(event string) {
	for _, l := range b.Listeners {
		l.OnNotify(event)
	}
}
