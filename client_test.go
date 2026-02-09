package go_ha_client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(serverURL, WithToken("test-token"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
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

func TestPing(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestNewClientInvalidURL(t *testing.T) {
	t.Parallel()

	_, err := NewClient(":", WithToken("token"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewClientDefaultTimeout(t *testing.T) {
	t.Parallel()

	client, err := NewClient("http://example.com", WithToken("token"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.httpClient.Timeout != defaultHTTPTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultHTTPTimeout, client.httpClient.Timeout)
	}
}

func TestNewClientWithTimeout(t *testing.T) {
	t.Parallel()

	client, err := NewClient("http://example.com", WithToken("token"), WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.httpClient.Timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %s", client.httpClient.Timeout)
	}
}

func TestNewClientWithTimeoutOverridesCustomHTTPClientTimeout(t *testing.T) {
	t.Parallel()

	base := &http.Client{
		Timeout: 2 * time.Second,
	}
	client, err := NewClient("http://example.com", WithToken("token"), WithHTTPClient(base), WithTimeout(7*time.Second))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.httpClient.Timeout != 7*time.Second {
		t.Fatalf("expected timeout 7s, got %s", client.httpClient.Timeout)
	}
	if base.Timeout != 2*time.Second {
		t.Fatalf("expected original client timeout untouched, got %s", base.Timeout)
	}
}

func TestWithLoggerOption(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client, err := NewClient("http://example.com", WithToken("token"), WithLogger(logger))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.config.Logger != logger {
		t.Fatalf("expected custom logger to be set")
	}
}

func TestWithDebugOptionEnablesDebugLevel(t *testing.T) {
	t.Parallel()

	client, err := NewClient("http://example.com", WithToken("token"), WithDebug())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if !client.config.Logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatalf("expected debug logger to be enabled")
	}
}

func TestGetHistoryNilQuery(t *testing.T) {
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
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	_, err := client.GetHistory(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetHistoryWithMinimalResponse(t *testing.T) {
	t.Parallel()

	start := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		expectedPath := "/api/history/period/" + start.Format(filterDateFormat)
		if r.URL.Path != expectedPath {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		if r.URL.Query().Get("minimal_response") != "true" {
			reportHandlerErr(errCh, fmt.Errorf("missing minimal_response query in %s", r.URL.RawQuery))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	query := NewHistoryQuery().WithStart(start).WithMinimalResponse(true)
	_, err := client.GetHistory(context.Background(), query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetConfig(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/config" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_ = json.NewEncoder(w).Encode(Config{
			Components:   []string{"light"},
			ConfigDir:    "/config",
			Elevation:    100,
			Latitude:     48.7,
			LocationName: "Home",
			Longitude:    21.2,
			TimeZone:     "Europe/Bratislava",
			Version:      "2025.1.0",
			WhitelistExternalDirs: []string{
				"/media",
			},
			AllowlistExternalDirs: []string{
				"/media",
			},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	cfg, err := client.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LocationName != "Home" || cfg.TimeZone != "Europe/Bratislava" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetEvents(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/events" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_ = json.NewEncoder(w).Encode(Events{
			{Event: "state_changed", ListenerCount: 1},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	events, err := client.GetEvents(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].Event != "state_changed" {
		t.Fatalf("unexpected events: %#v", events)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetServicesWithMap(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/services" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_ = json.NewEncoder(w).Encode(Services{
			{
				Domain: "light",
				Services: ServiceMap{
					"turn_on": {
						Name:        "Turn on",
						Description: "Turn on",
					},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	services, err := client.GetServices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) != 1 || services[0].Domain != "light" {
		t.Fatalf("unexpected services: %#v", services)
	}
	if _, ok := services[0].Services["turn_on"]; !ok {
		t.Fatalf("missing service: %#v", services[0].Services)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetServicesWithList(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/services" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		raw := []byte(`[{"domain":"light","services":["turn_on","turn_off"]}]`)
		_, _ = w.Write(raw)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	services, err := client.GetServices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) != 1 || services[0].Domain != "light" {
		t.Fatalf("unexpected services: %#v", services)
	}
	if _, ok := services[0].Services["turn_on"]; !ok {
		t.Fatalf("missing service: %#v", services[0].Services)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetStates(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/states" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_ = json.NewEncoder(w).Encode(StateEntities{
			{EntityID: "light.kitchen", State: "on"},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	states, err := client.GetStates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 1 || states[0].EntityID != "light.kitchen" {
		t.Fatalf("unexpected states: %#v", states)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetStateForEntity(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/states/light.kitchen" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_ = json.NewEncoder(w).Encode(StateEntity{
			EntityID: "light.kitchen",
			State:    "on",
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	state, err := client.GetStateForEntity(context.Background(), "light.kitchen")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.EntityID != "light.kitchen" || state.State != "on" {
		t.Fatalf("unexpected state: %#v", state)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetStateForEntityEscapesEntityID(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.EscapedPath() != "/api/states/light.kitchen%2F1" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.EscapedPath()))
			return
		}
		_ = json.NewEncoder(w).Encode(StateEntity{
			EntityID: "light.kitchen/1",
			State:    "on",
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	state, err := client.GetStateForEntity(context.Background(), "light.kitchen/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.EntityID != "light.kitchen/1" || state.State != "on" {
		t.Fatalf("unexpected state: %#v", state)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetLogbook(t *testing.T) {
	t.Parallel()

	start := time.Date(2021, 7, 1, 10, 20, 30, 0, time.UTC)
	end := time.Date(2021, 7, 2, 11, 22, 33, 0, time.UTC)

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/logbook/"+start.Format(filterDateFormat) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		if r.URL.Query().Get("end_time") != end.Format(filterDateFormat) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected end_time: %s", r.URL.Query().Get("end_time")))
			return
		}
		if r.URL.Query().Get("entity") != "light.kitchen" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected entity: %s", r.URL.Query().Get("entity")))
			return
		}
		_ = json.NewEncoder(w).Encode(LogbookRecords{
			{
				When:     time.Date(2021, 7, 1, 10, 30, 0, 0, time.UTC),
				Name:     "Kitchen",
				Message:  "turned on",
				Domain:   "light",
				EntityID: "light.kitchen",
			},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	records, err := client.GetLogbook(context.Background(), &LogbookFilter{
		StartTime: start,
		EndTime:   end,
		EntityID:  "light.kitchen",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 || records[0].EntityID != "light.kitchen" {
		t.Fatalf("unexpected logbook: %#v", records)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetPlainErrorLog(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/error_log" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_, _ = w.Write([]byte("log line"))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	log, err := client.GetPlainErrorLog(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(log) != "log line" {
		t.Fatalf("unexpected log: %s", log)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetCameraJpeg(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/camera_proxy/camera.kitchen" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	got, err := client.GetCameraJpeg(context.Background(), "camera.kitchen")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Bounds().Dx() != 2 || got.Bounds().Dy() != 2 {
		t.Fatalf("unexpected image size: %#v", got.Bounds())
	}
	assertNoHandlerErr(t, errCh)
}

func TestCreateStateSuccess(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/states/sensor.test" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected content-type: %s", ct))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			reportHandlerErr(errCh, fmt.Errorf("read body: %w", err))
			return
		}
		if !strings.Contains(string(body), `"state":"on"`) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected body: %s", string(body)))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(StateResponse{
			State: State{State: "on"},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	resp, err := client.CreateState(context.Background(), "sensor.test", State{State: "on"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Created() {
		t.Fatalf("expected Created response")
	}
	assertNoHandlerErr(t, errCh)
}

func TestCreateStateEmptyEntityID(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, "http://localhost")
	_, err := client.CreateState(context.Background(), "", State{State: "on"})
	if !errors.Is(err, ErrEmptyEntityID) {
		t.Fatalf("expected ErrEmptyEntityID, got: %v", err)
	}
}

func TestCallService(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/services/light/turn_on" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected content-type: %s", ct))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			reportHandlerErr(errCh, fmt.Errorf("read body: %w", err))
			return
		}
		if !strings.Contains(string(body), `"entity_id":"light.kitchen"`) {
			reportHandlerErr(errCh, fmt.Errorf("unexpected body: %s", string(body)))
			return
		}
		_ = json.NewEncoder(w).Encode(StateEntities{
			{EntityID: "light.kitchen", State: "on"},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	states, err := client.CallService(context.Background(), NewTurnLightOnCmd("light.kitchen"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 1 || states[0].EntityID != "light.kitchen" {
		t.Fatalf("unexpected states: %#v", states)
	}
	assertNoHandlerErr(t, errCh)
}

func TestCallServiceEscapesDomainAndService(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.EscapedPath() != "/api/services/light%2Fspecial/turn%20on" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.EscapedPath()))
			return
		}
		_ = json.NewEncoder(w).Encode(StateEntities{
			{EntityID: "light.kitchen", State: "on"},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	_, err := client.CallService(context.Background(), DefaultServiceCmd{
		Domain:   "light/special",
		Service:  "turn on",
		EntityID: "light.kitchen",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoHandlerErr(t, errCh)
}

func TestCallServiceWithResponse(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/services/light/turn_on" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		if r.URL.RawQuery != "return_response" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected query: %s", r.URL.RawQuery))
			return
		}
		_ = json.NewEncoder(w).Encode(ServiceCallResponse{
			ChangedStates: StateEntities{
				{EntityID: "light.kitchen", State: "on"},
			},
			ServiceResponse: map[string]json.RawMessage{
				"light.kitchen": json.RawMessage(`{"success":true}`),
			},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	resp, err := client.CallServiceWithResponse(context.Background(), "light", "turn_on", strings.NewReader(`{"entity_id":"light.kitchen"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ChangedStates) != 1 {
		t.Fatalf("unexpected changed states: %#v", resp.ChangedStates)
	}
	assertNoHandlerErr(t, errCh)
}

func TestRenderTemplate(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/template" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected content-type: %s", ct))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			reportHandlerErr(errCh, fmt.Errorf("read body: %w", err))
			return
		}
		if !strings.Contains(string(body), "states") {
			reportHandlerErr(errCh, fmt.Errorf("unexpected body: %s", string(body)))
			return
		}
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	rendered, err := client.RenderTemplate(context.Background(), "{{ states('sensor.test') }}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rendered != "hello" {
		t.Fatalf("unexpected rendered: %s", rendered)
	}
	assertNoHandlerErr(t, errCh)
}

func TestTriggerConfigCheck(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.Path != "/api/config/core/check_config" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.Path))
			return
		}
		_ = json.NewEncoder(w).Encode(ConfigurationCheckResult{
			Result: "valid",
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	result, err := client.TriggerConfigCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Result != "valid" {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertNoHandlerErr(t, errCh)
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

	client := newTestClient(t, server.URL)
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

	client := newTestClient(t, server.URL)
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

func TestFireEventEmptyEventType(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, "http://localhost")
	ok, err := client.FireEvent(context.Background(), "", nil)
	if ok {
		t.Fatalf("expected ok=false")
	}
	if !errors.Is(err, ErrEmptyEventType) {
		t.Fatalf("expected ErrEmptyEventType, got: %v", err)
	}
}

func TestCreateStatePropagatesError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
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

	client := newTestClient(t, server.URL)
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

	client := newTestClient(t, server.URL)
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
			{Name: "Home", EntityID: "calendar.home"},
		})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	calendars, err := client.GetCalendars(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calendars) != 1 || calendars[0].EntityID != "calendar.home" {
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

	client := newTestClient(t, server.URL)
	events, err := client.GetCalendarEvents(context.Background(), "calendar.home", start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].Summary != "Test" {
		t.Fatalf("unexpected events: %#v", events)
	}
	assertNoHandlerErr(t, errCh)
}

func TestGetCalendarEventsEscapesCalendarID(t *testing.T) {
	t.Parallel()

	start := time.Date(2021, 8, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2021, 8, 2, 11, 0, 0, 0, time.UTC)

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			reportHandlerErr(errCh, fmt.Errorf("unexpected method: %s", r.Method))
			return
		}
		if r.URL.EscapedPath() != "/api/calendars/calendar.home%2F1" {
			reportHandlerErr(errCh, fmt.Errorf("unexpected path: %s", r.URL.EscapedPath()))
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

	client := newTestClient(t, server.URL)
	events, err := client.GetCalendarEvents(context.Background(), "calendar.home/1", start, end)
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

	client := newTestClient(t, server.URL)
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

	client := newTestClient(t, server.URL)
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

type errorRoundTripper struct {
	err error
}

func (e errorRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, e.err
}

type readErrorReader struct {
	err error
}

func (r readErrorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

type readErrorBody struct {
	err error
}

func (b readErrorBody) Read(_ []byte) (int, error) {
	return 0, b.err
}

func (b readErrorBody) Close() error {
	return nil
}

type readErrorResponseRoundTripper struct {
	err error
}

func (rt readErrorResponseRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       readErrorBody(rt),
		Header:     make(http.Header),
	}, nil
}

func TestDoRequestWrapsRoundTripError(t *testing.T) {
	t.Parallel()

	client, err := NewClient("http://example.com", WithToken("token"), WithHTTPClient(&http.Client{
		Transport: errorRoundTripper{err: errors.New("network down")},
	}))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.GetConfig(context.Background())
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("expected wrapped error, got: %v", err)
	}
}

func TestDebugRequestBodyReadError(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"changed_states":[],"service_response":{}}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, WithToken("token"), WithDebug())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.CallServiceWithResponse(context.Background(), "light", "turn_on", readErrorReader{err: errors.New("read body failed")})
	if err == nil || !strings.Contains(err.Error(), "reading request body for debug") {
		t.Fatalf("expected debug request body read error, got: %v", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("expected request not to be sent on debug body read error")
	}
}

func TestDebugResponseBodyReadError(t *testing.T) {
	t.Parallel()

	client, err := NewClient("http://example.com", WithToken("token"), WithDebug(), WithHTTPClient(&http.Client{
		Transport: readErrorResponseRoundTripper{err: errors.New("read response failed")},
	}))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	err = client.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reading response body for debug") {
		t.Fatalf("expected debug response body read error, got: %v", err)
	}
}

func TestDebugModeRequestPaths(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/states/"):
			if r.Header.Get("Content-Type") != mimeTypeJSON {
				reportHandlerErr(errCh, fmt.Errorf("missing content type"))
				return
			}
			_ = json.NewEncoder(w).Encode(StateResponse{
				EntityID: "light.kitchen",
				State: State{
					State: "on",
				},
			})
		default:
			reportHandlerErr(errCh, fmt.Errorf("unexpected request: %s %s", r.Method, r.URL.Path))
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, WithToken("token"), WithDebug())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}

	_, err = client.CreateState(context.Background(), "light.kitchen", State{State: "on"})
	if err != nil {
		t.Fatalf("create state: %v", err)
	}

	assertNoHandlerErr(t, errCh)
}

func TestBadRequestFallbackError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	if err := client.Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("expected bad request error, got: %v", err)
	}
}

func TestUnauthorizedError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	if err := client.Ping(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestNotFoundError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	if err := client.Ping(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestGenericHttpErrorIncludesResponseBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal boom"))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "internal boom") {
		t.Fatalf("expected status and body in error, got: %v", err)
	}
}

func TestGenericHttpErrorTruncatesLongResponseBody(t *testing.T) {
	t.Parallel()

	longBody := strings.Repeat("a", 1024) + "TRUNCATE_MARKER"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(longBody))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "500") {
		t.Fatalf("expected status code in error, got: %v", err)
	}
	if strings.Contains(errMsg, "TRUNCATE_MARKER") {
		t.Fatalf("expected truncated body, got: %v", err)
	}
}
