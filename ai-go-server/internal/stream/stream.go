// Package stream provides context-aware helpers for consuming channels.
package stream

import "context"

// Process passes every item from ch to handler until the channel closes, the
// context is cancelled, or the handler returns an error.
func Process[T any](ctx context.Context, ch <-chan T, handler func(T) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case item, ok := <-ch:
			if !ok {
				return nil
			}
			if err := handler(item); err != nil {
				return err
			}
		}
	}
}

// Collect gathers all values from ch while observing ctx cancellation.
func Collect[T any](ctx context.Context, ch <-chan T) ([]T, error) {
	var values []T
	err := Process(ctx, ch, func(value T) error {
		values = append(values, value)
		return nil
	})
	return values, err
}
