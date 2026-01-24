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
		FilterEntityId:         "light.kitchen",
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
