package go_ha_client

import (
	"context"
	"strings"
	"time"
)

// HistoryQuery builds filters for the history endpoint.
type HistoryQuery struct {
	filter StateChangesFilter
}

// NewHistoryQuery creates a new history query builder.
func NewHistoryQuery() *HistoryQuery {
	return &HistoryQuery{}
}

// WithStart sets the start time.
func (q *HistoryQuery) WithStart(t time.Time) *HistoryQuery {
	q.filter.StartTime = t
	return q
}

// WithEnd sets the end time.
func (q *HistoryQuery) WithEnd(t time.Time) *HistoryQuery {
	q.filter.EndTime = t
	return q
}

// WithEntities sets filter_entity_id with comma-separated entity IDs.
func (q *HistoryQuery) WithEntities(entityIDs ...string) *HistoryQuery {
	q.filter.FilterEntityID = strings.Join(entityIDs, ",")
	return q
}

// WithMinimalResponse sets minimal_response.
func (q *HistoryQuery) WithMinimalResponse(enabled bool) *HistoryQuery {
	q.filter.MinimalResponse = enabled
	return q
}

// WithNoAttributes sets no_attributes.
func (q *HistoryQuery) WithNoAttributes(enabled bool) *HistoryQuery {
	q.filter.NoAttributes = enabled
	return q
}

// WithSignificantChangesOnly sets significant_changes_only.
func (q *HistoryQuery) WithSignificantChangesOnly(enabled bool) *HistoryQuery {
	q.filter.SignificantChangesOnly = enabled
	return q
}

// Filter returns a copy of the filter for use with GetStateChangesHistory.
func (q *HistoryQuery) Filter() *StateChangesFilter {
	filter := q.filter
	return &filter
}

// String renders the query as a path + query string.
func (q *HistoryQuery) String() string {
	return q.filter.String()
}

// GetHistory retrieves state changes history using a HistoryQuery.
func (c *Client) GetHistory(ctx context.Context, query *HistoryQuery) (StateChanges, error) {
	if query == nil {
		return c.GetStateChangesHistory(ctx, nil)
	}
	return c.GetStateChangesHistory(ctx, query.Filter())
}
