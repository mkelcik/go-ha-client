package go_ha_client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(serverURL string) *Client {
	return NewClient(ClientConfig{
		Token: "test-token",
		Host:  serverURL,
	}, &http.Client{})
}

func reportHandlerErr(errCh chan error, err error) {
	if err == nil {
		return
	}
	select {
	case errCh <- err:
	default:
	}
}

func assertNoHandlerErr(t *testing.T, errCh chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		t.Fatalf("handler error: %v", err)
	default:
	}
}

func TestGetStateChangesHistoryNilFilter(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/history/period" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_ = json.NewEncoder(w).Encode(StateChanges{})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	_, err := client.GetStateChangesHistory(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestFireEventWithTimeSendsBody(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/events/sunrise" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			reportHandlerErr(errCh, fmt.Errorf("read body: %w", err))
			return
		}
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	at := time.Date(2021, 1, 2, 3, 4, 5, 0, time.UTC)
	ok, err := client.FireEvent(context.Background(), "sunrise", &at)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if !strings.Contains(gotBody, "next_rising") {
		t.Fatalf("expected next_rising in body, got: %s", gotBody)
	}
	assertNoHandlerErr(t, errCh)
}

func TestCreateStatePropagatesError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	_, err := client.CreateState(context.Background(), "sensor.test", State{State: "on"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestGetComponents(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/components" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_ = json.NewEncoder(w).Encode([]string{"light", "switch"})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	components, err := client.GetComponents(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(components) != 2 || components[0] != "light" || components[1] != "switch" {
		t.Fatalf("unexpected components: %#v", components)
	}
	assertNoHandlerErr(t, errCh)
}

func TestDeleteState(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/states/sensor.test" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	if err := client.DeleteState(context.Background(), "sensor.test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetCalendars(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/calendars" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_ = json.NewEncoder(w).Encode(Calendars{
			{Name: "Home", EntityId: "calendar.home"},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	calendars, err := client.GetCalendars(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calendars) != 1 || calendars[0].EntityId != "calendar.home" {
		t.Fatalf("unexpected calendars: %#v", calendars)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetCalendarEvents(t *testing.T) {
	t.Parallel()

	start := time.Date(2021, 8, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2021, 8, 2, 11, 0, 0, 0, time.UTC)

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/calendars/calendar.home" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		if r.URL.Query().Get("start") != start.Format(time.RFC3339) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected start: %s", r.URL.Query().Get("start")))
			return
		}
		if r.URL.Query().Get("end") != end.Format(time.RFC3339) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected end: %s", r.URL.Query().Get("end")))
			return
		}
		_ = json.NewEncoder(w).Encode(CalendarEvents{
			{
				Summary: "Test",
				Start:   CalendarEventTime{DateTime: "2021-08-01T10:00:00Z"},
				End:     CalendarEventTime{DateTime: "2021-08-01T11:00:00Z"},
			},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	events, err := client.GetCalendarEvents(context.Background(), "calendar.home", start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].Summary != "Test" {
		t.Fatalf("unexpected events: %#v", events)
	}
	assertNoHandlerErr(t, errCh)
}

func TestHandleIntent(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/intent/handle" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			reportHandlerErr(errCh, fmt.Errorf("read body: %w", err))
			return
		}
		if !strings.Contains(string(body), `"name":"TurnOn"`) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected body: %s", string(body)))
			return
		}
		_ = json.NewEncoder(w).Encode(IntentResponse{
			Response: map[string]interface{}{
				"speech": map[string]interface{}{
					"plain": map[string]interface{}{
						"speech": "ok",
					},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	resp, err := client.HandleIntent(context.Background(), IntentRequest{
		Name: "TurnOn",
		Data: map[string]interface{}{"entity": "light.kitchen"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Response == nil {
		t.Fatalf("expected response data")
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetWeatherForecasts(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/services/weather/get_forecasts" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		if r.URL.RawQuery != "return_response" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected query: %s", r.URL.RawQuery))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			reportHandlerErr(errCh, fmt.Errorf("read body: %w", err))
			return
		}
		if !strings.Contains(string(body), `"entity_id":"weather.home"`) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected body: %s", string(body)))
			return
		}
		if !strings.Contains(string(body), `"type":"daily"`) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected body: %s", string(body)))
			return
		}
		_ = json.NewEncoder(w).Encode(ServiceCallResponse{
			ChangedStates: StateEntities{},
			ServiceResponse: map[string]json.RawMessage{
				"weather.home": json.RawMessage(`{"forecast":[{"condition":"sunny","datetime":"2021-01-01T00:00:00Z"}]}`),
			},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	forecasts, err := client.GetWeatherForecasts(context.Background(), "weather.home", "daily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(forecasts.Forecast) != 1 {
		t.Fatalf("unexpected forecast count: %#v", forecasts.Forecast)
	}
	if forecasts.Forecast[0]["condition"] != "sunny" {
		t.Fatalf("unexpected forecast content: %#v", forecasts.Forecast[0])
	}
	assertNoHandlerErr(t, errCh)
}

func TestProcessConversation(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/conversation/process" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			reportHandlerErr(errCh, fmt.Errorf("read body: %w", err))
			return
		}
		if !strings.Contains(string(body), `"text":"Turn on kitchen lights"`) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected body: %s", string(body)))
			return
		}
		_ = json.NewEncoder(w).Encode(ConversationProcessResponse{
			ContinueConversation: false,
			ConversationId:       "conv-1",
			Response: ConversationResponse{
				ResponseType: "action_done",
				Language:     "en",
				Speech: map[string]interface{}{
					"plain": map[string]interface{}{
						"speech": "Done",
					},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	resp, err := client.ProcessConversation(context.Background(), ConversationProcessRequest{
		Text:     "Turn on kitchen lights",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Response.ResponseType != "action_done" {
		t.Fatalf("unexpected response: %#v", resp.Response)
	}
	assertNoHandlerErr(t, errCh)
}
