package go_ha_client

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCreateQueryStringStartTimeOnly(t *testing.T) {
	t.Parallel()

	start := time.Date(2021, 7, 1, 10, 20, 30, 0, time.FixedZone("UTC+2", 2*60*60))
	filter := &StateChangesFilter{
		StartTime: start,
	}

	got := createQueryString(filter.StartTime, filter)
	want := "/" + start.Format(filterDateFormat)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestCreateQueryStringWithParams(t *testing.T) {
	t.Parallel()

	start := time.Date(2021, 7, 1, 10, 20, 30, 0, time.UTC)
	end := time.Date(2021, 7, 2, 11, 22, 33, 0, time.UTC)
	filter := &StateChangesFilter{
		StartTime:              start,
		EndTime:                end,
		FilterEntityID:         "light.kitchen",
		MinimalResponse:        true,
		NoAttributes:           true,
		SignificantChangesOnly: true,
	}

	got := createQueryString(filter.StartTime, filter)
	if !strings.HasPrefix(got, "/"+start.Format(filterDateFormat)+"?") {
		t.Fatalf("unexpected prefix: %s", got)
	}

	escapedEnd := url.QueryEscape(end.Format(filterDateFormat))
	expectedParts := []string{
		"end_time=" + escapedEnd,
		"filter_entity_id=light.kitchen",
		"minimal_response=true",
		"no_attributes=true",
		"significant_changes_only=true",
	}
	for _, part := range expectedParts {
		if !strings.Contains(got, part) {
			t.Fatalf("missing part %q in %q", part, got)
		}
	}
}

func TestLogbookFilterStringWithParams(t *testing.T) {
	t.Parallel()

	start := time.Date(2023, 3, 1, 9, 0, 0, 0, time.UTC)
	end := time.Date(2023, 3, 2, 10, 30, 0, 0, time.UTC)
	filter := &LogbookFilter{
		StartTime: start,
		EndTime:   end,
		EntityID:  "light.kitchen",
	}

	got := filter.String()
	if !strings.HasPrefix(got, "/"+start.Format(filterDateFormat)+"?") {
		t.Fatalf("unexpected prefix: %s", got)
	}

	escapedEnd := url.QueryEscape(end.Format(filterDateFormat))
	if !strings.Contains(got, "end_time="+escapedEnd) {
		t.Fatalf("missing end_time in %q", got)
	}
	if !strings.Contains(got, "entity=light.kitchen") {
		t.Fatalf("missing entity in %q", got)
	}
}

func TestServiceMapUnmarshalList(t *testing.T) {
	t.Parallel()

	raw := []byte(`["turn_on","turn_off"]`)
	var m ServiceMap
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := m["turn_on"]; !ok {
		t.Fatalf("missing turn_on")
	}
	if m["turn_on"].Name != "turn_on" {
		t.Fatalf("unexpected service: %#v", m["turn_on"])
	}
}

func TestServiceMapUnmarshalMap(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"turn_on":{"name":"Turn On","description":"Turn on"}}`)
	var m ServiceMap
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := m["turn_on"]; !ok {
		t.Fatalf("missing turn_on")
	}
	if m["turn_on"].Description != "Turn on" {
		t.Fatalf("unexpected service: %#v", m["turn_on"])
	}
}

func TestServiceMapUnmarshalNull(t *testing.T) {
	t.Parallel()

	raw := []byte(`null`)
	m := ServiceMap{
		"turn_on": {Name: "Turn On"},
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Fatalf("expected nil map, got: %#v", m)
	}
}

func TestNewServiceDataEntityID(t *testing.T) {
	t.Parallel()

	data := NewServiceDataEntityID("light.kitchen")
	v, ok := data["entity_id"]
	if !ok {
		t.Fatalf("missing entity_id key")
	}
	if v != "light.kitchen" {
		t.Fatalf("unexpected entity_id value: %#v", v)
	}
}

func TestBuildAndParseEntityID(t *testing.T) {
	t.Parallel()

	id := BuildEntityID("light", "kitchen")
	if id != "light.kitchen" {
		t.Fatalf("unexpected entity id: %s", id)
	}

	domain, objectID, err := ParseEntityID(id)
	if err != nil {
		t.Fatalf("parse entity id: %v", err)
	}
	if domain != "light" || objectID != "kitchen" {
		t.Fatalf("unexpected parsed values: %s %s", domain, objectID)
	}
}

func TestParseEntityIDInvalid(t *testing.T) {
	t.Parallel()

	cases := []string{
		"light",
		"light.",
		".kitchen",
		"light.kitchen.extra",
	}
	for _, tc := range cases {
		if _, _, err := ParseEntityID(tc); err == nil {
			t.Fatalf("expected error for %q", tc)
		}
	}
}

func TestHistoryQueryBuilder(t *testing.T) {
	t.Parallel()

	start := time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC)
	end := time.Date(2022, 1, 3, 4, 5, 6, 0, time.UTC)

	query := NewHistoryQuery().
		WithStart(start).
		WithEnd(end).
		WithEntities("light.kitchen", "sensor.temp").
		WithMinimalResponse(true).
		WithNoAttributes(true).
		WithSignificantChangesOnly(true)

	filter := query.Filter()
	if filter.StartTime != start {
		t.Fatalf("unexpected start time: %v", filter.StartTime)
	}
	if filter.EndTime != end {
		t.Fatalf("unexpected end time: %v", filter.EndTime)
	}
	if filter.FilterEntityID != "light.kitchen,sensor.temp" {
		t.Fatalf("unexpected entity filter: %s", filter.FilterEntityID)
	}
	if !filter.MinimalResponse || !filter.NoAttributes || !filter.SignificantChangesOnly {
		t.Fatalf("unexpected flags: minimal_response=%t no_attributes=%t significant_changes_only=%t", filter.MinimalResponse, filter.NoAttributes, filter.SignificantChangesOnly)
	}

	got := query.String()
	escapedEnd := url.QueryEscape(end.Format(filterDateFormat))
	escapedEntities := url.QueryEscape("light.kitchen,sensor.temp")
	if !strings.Contains(got, "end_time="+escapedEnd) {
		t.Fatalf("missing end_time in %q", got)
	}
	if !strings.Contains(got, "filter_entity_id="+escapedEntities) {
		t.Fatalf("missing filter_entity_id in %q", got)
	}
	if !strings.Contains(got, "no_attributes=true") {
		t.Fatalf("missing no_attributes in %q", got)
	}
	if !strings.Contains(got, "minimal_response=true") {
		t.Fatalf("missing minimal_response in %q", got)
	}
	if !strings.Contains(got, "significant_changes_only=true") {
		t.Fatalf("missing significant_changes_only in %q", got)
	}
}
