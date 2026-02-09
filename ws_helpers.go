package go_ha_client

import (
	"context"
	"encoding/json"
	"errors"
)

// SubscribeStateChanged subscribes to state changes for a single entity.
func (c *WSClient) SubscribeStateChanged(ctx context.Context, entityID string) (*WSSubscription, error) {
	return c.SubscribeStateChangedMany(ctx, entityID)
}

// SubscribeStateChangedMany subscribes to state changes for multiple entities.
func (c *WSClient) SubscribeStateChangedMany(ctx context.Context, entityIDs ...string) (*WSSubscription, error) {
	if len(entityIDs) == 0 {
		return nil, ErrEmptyEntityID
	}

	allowedEntities := make(map[string]struct{}, len(entityIDs))
	for _, entityID := range entityIDs {
		if entityID == "" {
			return nil, ErrEmptyEntityID
		}
		allowedEntities[entityID] = struct{}{}
	}

	base, err := c.SubscribeEvents(ctx, EventTypeStateChanged)
	if err != nil {
		return nil, err
	}
	return filterSubscriptionByEntities(base, allowedEntities), nil
}

// WaitForState blocks until predicate returns true for the given entity state.
func (c *WSClient) WaitForState(ctx context.Context, entityID string, predicate func(State) bool) error {
	if entityID == "" {
		return ErrEmptyEntityID
	}
	if predicate == nil {
		return errors.New("predicate must not be nil")
	}

	sub, err := c.SubscribeStateChanged(ctx, entityID)
	if err != nil {
		return err
	}
	defer func() {
		unsubCtx, cancel := context.WithTimeout(context.Background(), wsUnsubscribeTimeout)
		defer cancel()
		_ = sub.Unsubscribe(unsubCtx)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-sub.Errors():
			if !ok {
				return ErrWSClosed
			}
			if err != nil {
				return err
			}
		case ev, ok := <-sub.Events():
			if !ok {
				return ErrWSClosed
			}
			data, ok, err := ev.StateChanged()
			if err != nil {
				continue
			}
			if !ok {
				continue
			}
			if data.NewState == nil {
				continue
			}
			if predicate(*data.NewState) {
				return nil
			}
		}
	}
}

// WaitForStateEquals blocks until the entity state matches the target state string.
func (c *WSClient) WaitForStateEquals(ctx context.Context, entityID, targetState string) error {
	return c.WaitForState(ctx, entityID, func(s State) bool {
		return s.State == targetState
	})
}

// CallServiceForEntity calls a service with service_data.entity_id prefilled.
func (c *WSClient) CallServiceForEntity(ctx context.Context, domain, service, entityID string, data map[string]interface{}) (WSCallServiceResult, error) {
	result := WSCallServiceResult{}
	if entityID == "" {
		return result, ErrEmptyEntityID
	}

	merged := make(map[string]interface{}, len(data)+1)
	for k, v := range data {
		merged[k] = v
	}
	merged["entity_id"] = entityID
	return c.CallService(ctx, domain, service, merged)
}

// DecodeEventData decodes event.Data into the provided type.
func DecodeEventData[T any](event WSEvent) (T, error) {
	var data T
	if len(event.Data) == 0 {
		return data, errors.New("event data is empty")
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return data, err
	}
	return data, nil
}

func filterSubscriptionByEntities(sub *WSSubscription, allowedEntities map[string]struct{}) *WSSubscription {
	filtered := &WSSubscription{
		id:       sub.ID(),
		idSource: &sub.id,
		events:   make(chan WSEvent, 32),
		errors:   make(chan error, 1),
		client:   sub.client,
	}

	go func() {
		defer filtered.once.Do(func() {
			close(filtered.events)
			close(filtered.errors)
		})

		eventsCh := sub.Events()
		errorsCh := sub.Errors()
		for eventsCh != nil || errorsCh != nil {
			select {
			case ev, ok := <-eventsCh:
				if !ok {
					eventsCh = nil
					continue
				}
				if ev.EventType != EventTypeStateChanged {
					continue
				}
				payload, ok, err := ev.StateChanged()
				if err != nil || !ok {
					continue
				}
				if _, ok := allowedEntities[payload.EntityID]; !ok {
					continue
				}
				select {
				case filtered.events <- ev:
				default:
					select {
					case filtered.errors <- errors.New("ws subscription event buffer is full"):
					default:
					}
				}
			case err, ok := <-errorsCh:
				if !ok {
					errorsCh = nil
					continue
				}
				if err == nil {
					continue
				}
				select {
				case filtered.errors <- err:
				default:
				}
			}
		}
	}()

	return filtered
}
