package eventbus

import (
	"context"

	"github.com/ridesafety/auth-service/internal/domain/events"
)

type EventBus interface {
	Publish(ctx context.Context, event events.DomainEvent) error
}
