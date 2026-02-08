package go_ha_client

import (
	"context"
	"encoding/json"
	"errors"
)

type stateChangedData struct {
	EntityID string `json:"entity_id"`
	NewState State  `json:"new_state"`
}

// SubscribeStateChanged subscribes to state changes for a single entity.
func (c *WSClient) SubscribeStateChanged(ctx context.Context, entityID string) (*WSSubscription, error) {
	if entityID == "" {
		return nil, ErrEmptyEntityID
	}
	base, err := c.SubscribeEvents(ctx, EventTypeStateChanged)
	if err != nil {
		return nil, err
	}
	return filterSubscriptionByEntity(base, entityID), nil
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
			data, err := DecodeEventData[stateChangedData](ev)
			if err != nil {
				continue
			}
			if predicate(data.NewState) {
				return nil
			}
		}
	}
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

func filterSubscriptionByEntity(sub *WSSubscription, entityID string) *WSSubscription {
	filtered := &WSSubscription{
		id:     sub.id,
		events: make(chan WSEvent, 32),
		errors: make(chan error, 1),
		client: sub.client,
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
				payload, err := DecodeEventData[stateChangedData](ev)
				if err != nil || payload.EntityID != entityID {
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
