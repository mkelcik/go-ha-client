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
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	epPing                = "/api/"
	epConfig              = "/api/config"
	epComponents          = "/api/components"
	epEvents              = "/api/events"
	epServices            = "/api/services"
	epHistoryStateChanges = "/api/history/period"
	epLogbook             = "/api/logbook"
	epCalendars           = "/api/calendars"
	epState               = "/api/states"
	epStateEntity         = "/api/states/%s"
	epPlainErrorLog       = "/api/error_log"
	epCameraProxy         = "/api/camera_proxy/%s"
	epCallService         = "/api/services/%s/%s"
	epFireEvent           = "/api/events/%s"
	epTemplate            = "/api/template"
	epIntentHandle        = "/api/intent/handle"
	epConfigurationCheck  = "/api/config/core/check_config"
)

const (
	headerContentType   = "Content-Type"
	headerAuthorization = "Authorization"
	mimeTypeJSON        = "application/json"
	defaultHTTPTimeout  = 30 * time.Second
)

// Common errors returned by the client.
var (
	ErrNotFound        = errors.New("not found")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrEmptyEntityID   = errors.New("entityID must not be empty")
	ErrEmptyCalendarID = errors.New("calendarID must not be empty")
	ErrEmptyTemplate   = errors.New("empty template")
	ErrEmptyService    = errors.New("empty service name")
	ErrEmptyDomain     = errors.New("empty domain name")
	ErrEmptyEventType  = errors.New("eventType must not be empty")
	ErrEmptyIntentName = errors.New("empty intent name")
)

type badRequestResponse struct {
	Message string `json:"message"`
}

// ClientConfig holds configuration for the Client.
type ClientConfig struct {
	Token      string
	Host       string
	Logger     *slog.Logger
	HTTPClient *http.Client
	Timeout    time.Duration
	timeoutSet bool
}

// Client is a client for Home Assistant REST API.
type Client struct {
	config     ClientConfig
	httpClient *http.Client
}

// Option is a function that configures the Client.
type Option func(*ClientConfig)

// WithToken sets the access token for the client.
func WithToken(token string) Option {
	return func(c *ClientConfig) {
		c.Token = token
	}
}

// WithLogger sets the logger for the client.
func WithLogger(logger *slog.Logger) Option {
	return func(c *ClientConfig) {
		c.Logger = logger
	}
}

// WithHTTPClient sets the HTTP client to use.
func WithHTTPClient(client *http.Client) Option {
	return func(c *ClientConfig) {
		c.HTTPClient = client
	}
}

// WithTimeout sets the HTTP timeout used by the client.
func WithTimeout(timeout time.Duration) Option {
	return func(c *ClientConfig) {
		c.Timeout = timeout
		c.timeoutSet = true
	}
}

// WithDebug enables debug logging (using default text handler to stderr with debug level).
func WithDebug() Option {
	return func(c *ClientConfig) {
		c.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
}

// NewClient creates a new Client.
func NewClient(host string, opts ...Option) (*Client, error) {
	if _, err := url.ParseRequestURI(host); err != nil {
		return nil, fmt.Errorf("invalid host url: %w", err)
	}
	host = strings.TrimRight(host, "/")

	config := ClientConfig{
		Host:    host,
		Logger:  slog.Default(),
		Timeout: defaultHTTPTimeout,
	}

	for _, opt := range opts {
		opt(&config)
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	} else if config.timeoutSet {
		cloned := *httpClient
		cloned.Timeout = config.Timeout
		httpClient = &cloned
	}

	return &Client{
		config:     config,
		httpClient: httpClient,
	}, nil
}

// Ping checks if the API is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.doGetRequestJson(ctx, epPing, nil)
}

// GetConfig retrieves the current configuration.
func (c *Client) GetConfig(ctx context.Context) (Config, error) {
	config := Config{}
	return config, c.doGetRequestJson(ctx, epConfig, &config)
}

// GetComponents retrieves the list of loaded components.
func (c *Client) GetComponents(ctx context.Context) ([]string, error) {
	components := []string{}
	return components, c.doGetRequestJson(ctx, epComponents, &components)
}

// GetEvents retrieves the list of events.
func (c *Client) GetEvents(ctx context.Context) (Events, error) {
	events := Events{}
	return events, c.doGetRequestJson(ctx, epEvents, &events)
}

// GetServices retrieves the list of services.
func (c *Client) GetServices(ctx context.Context) (Services, error) {
	services := Services{}
	return services, c.doGetRequestJson(ctx, epServices, &services)
}

// GetStateChangesHistory retrieves state changes history.
func (c *Client) GetStateChangesHistory(ctx context.Context, filter *StateChangesFilter) (StateChanges, error) {
	changes := StateChanges{}
	filterString := ""
	if filter != nil {
		filterString = filter.String()
	}
	return changes, c.doGetRequestJson(ctx, epHistoryStateChanges+filterString, &changes)
}

// GetStates retrieves the list of all states.
func (c *Client) GetStates(ctx context.Context) (StateEntities, error) {
	states := StateEntities{}
	return states, c.doGetRequestJson(ctx, epState, &states)
}

// GetStateForEntity retrieves the state for a specific entity.
func (c *Client) GetStateForEntity(ctx context.Context, entityID string) (StateEntity, error) {
	state := StateEntity{}
	if entityID == "" {
		return state, ErrEmptyEntityID
	}
	return state, c.doGetRequestJson(ctx, fmt.Sprintf(epStateEntity, url.PathEscape(entityID)), &state)
}

// DeleteState deletes a state.
func (c *Client) DeleteState(ctx context.Context, entityID string) error {
	if entityID == "" {
		return ErrEmptyEntityID
	}
	return c.doRequest(ctx, http.MethodDelete, fmt.Sprintf(epStateEntity, url.PathEscape(entityID)), nil, func(reader io.Reader) error {
		return nil
	})
}

// GetLogbook retrieves logbook entries.
func (c *Client) GetLogbook(ctx context.Context, filter *LogbookFilter) (LogbookRecords, error) {
	logbookRecords := LogbookRecords{}
	filterString := ""
	if filter != nil {
		filterString = filter.String()
	}
	return logbookRecords, c.doGetRequestJson(ctx, epLogbook+filterString, &logbookRecords)
}

// GetCalendars retrieves the list of calendars.
func (c *Client) GetCalendars(ctx context.Context) (Calendars, error) {
	calendars := Calendars{}
	return calendars, c.doGetRequestJson(ctx, epCalendars, &calendars)
}

// GetCalendarEvents retrieves events for a specific calendar.
func (c *Client) GetCalendarEvents(ctx context.Context, calendarID string, start, end time.Time) (CalendarEvents, error) {
	events := CalendarEvents{}
	if calendarID == "" {
		return events, ErrEmptyCalendarID
	}

	query := url.Values{}
	if !start.IsZero() {
		query.Set("start", start.Format(time.RFC3339))
	}
	if !end.IsZero() {
		query.Set("end", end.Format(time.RFC3339))
	}

	queryString := ""
	if len(query) > 0 {
		queryString = "?" + query.Encode()
	}

	endpoint := fmt.Sprintf("%s/%s%s", epCalendars, url.PathEscape(calendarID), queryString)
	return events, c.doGetRequestJson(ctx, endpoint, &events)
}

// GetPlainErrorLog retrieves the error log as plain text.
func (c *Client) GetPlainErrorLog(ctx context.Context) (PlainText, error) {
	plaintText := ""
	return PlainText(plaintText), c.doGetRequestPlain(ctx, epPlainErrorLog, &plaintText)
}

// GetCameraJpeg retrieves a camera image.
func (c *Client) GetCameraJpeg(ctx context.Context, cameraEntityID string) (image.Image, error) {
	if cameraEntityID == "" {
		return nil, ErrEmptyEntityID
	}
	var img image.Image
	return img, c.doRequest(ctx, http.MethodGet, fmt.Sprintf(epCameraProxy, url.PathEscape(cameraEntityID)), nil, func(reader io.Reader) error {
		var err error
		img, err = jpeg.Decode(reader)
		if err != nil {
			return err
		}
		return nil
	})
}

// CreateState creates or updates a state.
func (c *Client) CreateState(ctx context.Context, entityID string, newState State) (StateResponse, error) {
	response := StateResponse{}
	if entityID == "" {
		return response, ErrEmptyEntityID
	}
	b, err := json.Marshal(newState)
	if err != nil {
		return response, fmt.Errorf("error creating state request body: %w", err)
	}

	respCode, err := c.doWithHeaders(ctx, http.MethodPost, fmt.Sprintf(epStateEntity, url.PathEscape(entityID)), bytes.NewBuffer(b), map[string]string{
		headerContentType: mimeTypeJSON,
	}, func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&response)
	})

	if respCode != nil {
		response.CreateCode = *respCode
	}
	return response, err
}

// CallService calls a service.
func (c *Client) CallService(ctx context.Context, cmd DefaultServiceCmd) (StateEntities, error) {
	states := StateEntities{}

	if cmd.Service == "" {
		return states, ErrEmptyService
	}

	if cmd.Domain == "" {
		return states, ErrEmptyDomain
	}

	return states, c.doPostRequestJson(ctx, fmt.Sprintf(epCallService, url.PathEscape(cmd.Domain), url.PathEscape(cmd.Service)), cmd.Reader(), &states)
}

// CallServiceWithResponse calls a service and returns the response.
func (c *Client) CallServiceWithResponse(ctx context.Context, domain, service string, body io.Reader) (ServiceCallResponse, error) {
	response := ServiceCallResponse{}

	if service == "" {
		return response, ErrEmptyService
	}

	if domain == "" {
		return response, ErrEmptyDomain
	}

	endpoint := fmt.Sprintf("%s?return_response", fmt.Sprintf(epCallService, url.PathEscape(domain), url.PathEscape(service)))
	return response, c.doPostRequestJson(ctx, endpoint, body, &response)
}

// FireEvent fires an event.
func (c *Client) FireEvent(ctx context.Context, eventType string, atTime *time.Time) (bool, error) {
	if eventType == "" {
		return false, ErrEmptyEventType
	}

	reader := &bytes.Buffer{}
	if atTime != nil {
		b, err := json.Marshal(newEventRising{NextRising: atTime})
		if err != nil {
			return false, fmt.Errorf("error creating event request body: %w", err)
		}
		reader = bytes.NewBuffer(b)
	}
	if err := c.doPostRequestJson(ctx, fmt.Sprintf(epFireEvent, url.PathEscape(eventType)), reader, nil); err != nil {
		return false, err
	}
	return true, nil
}

// RenderTemplate renders a template.
func (c *Client) RenderTemplate(ctx context.Context, template string) (string, error) {
	if template == "" {
		return "", ErrEmptyTemplate
	}

	b, err := json.Marshal(templateRequest{
		Template: template,
	})
	if err != nil {
		return "", fmt.Errorf("error creating template request: %w", err)
	}

	rendered := ""
	return rendered, c.doRequestWithHeaders(ctx, http.MethodPost, epTemplate, bytes.NewBuffer(b), map[string]string{
		headerContentType: mimeTypeJSON,
	}, func(reader io.Reader) error {
		tmp, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		rendered = string(tmp)
		return nil
	})
}

// HandleIntent handles an intent.
func (c *Client) HandleIntent(ctx context.Context, req IntentRequest) (IntentResponse, error) {
	response := IntentResponse{}
	if req.Name == "" {
		return response, ErrEmptyIntentName
	}

	b, err := json.Marshal(req)
	if err != nil {
		return response, fmt.Errorf("error creating intent request: %w", err)
	}

	return response, c.doPostRequestJson(ctx, epIntentHandle, bytes.NewBuffer(b), &response)
}

// GetWeatherForecasts retrieves weather forecasts.
func (c *Client) GetWeatherForecasts(ctx context.Context, entityID, forecastType string) (WeatherForecasts, error) {
	forecasts := WeatherForecasts{}
	if entityID == "" {
		return forecasts, ErrEmptyEntityID
	}

	b, err := json.Marshal(WeatherForecastRequest{
		EntityID: entityID,
		Type:     forecastType,
	})
	if err != nil {
		return forecasts, fmt.Errorf("error creating weather forecast request: %w", err)
	}

	resp, err := c.CallServiceWithResponse(ctx, DomainWeather, ServiceGetForecast, bytes.NewBuffer(b))
	if err != nil {
		return forecasts, err
	}

	raw, ok := resp.ServiceResponse[entityID]
	if !ok {
		return forecasts, fmt.Errorf("missing service response for entity `%s`", entityID)
	}

	if err := json.Unmarshal(raw, &forecasts); err != nil {
		return forecasts, fmt.Errorf("error decoding weather forecast response: %w", err)
	}
	return forecasts, nil
}

// TriggerConfigCheck triggers a configuration check.
func (c *Client) TriggerConfigCheck(ctx context.Context) (ConfigurationCheckResult, error) {
	result := ConfigurationCheckResult{}
	return result, c.doPostRequestJson(ctx, epConfigurationCheck, nil, &result)
}

func (c *Client) doGetRequestJson(ctx context.Context, endpoint string, responseEntity interface{}) error {
	return c.doRequest(ctx, http.MethodGet, endpoint, nil, func(reader io.Reader) error {
		if responseEntity == nil {
			return nil
		}

		if err := json.NewDecoder(reader).Decode(responseEntity); err != nil {
			return err
		}
		return nil
	})
}

func (c *Client) doPostRequestJson(ctx context.Context, endpoint string, body io.Reader, responseEntity interface{}) error {
	return c.doRequestWithHeaders(ctx, http.MethodPost, endpoint, body, map[string]string{
		headerContentType: mimeTypeJSON,
	}, func(reader io.Reader) error {
		if responseEntity == nil {
			return nil
		}

		if err := json.NewDecoder(reader).Decode(responseEntity); err != nil {
			return err
		}
		return nil
	})
}

func (c *Client) doGetRequestPlain(ctx context.Context, endpoint string, plainText *string) error {
	return c.doRequest(ctx, http.MethodGet, endpoint, nil, func(reader io.Reader) error {
		if plainText == nil {
			return nil
		}
		b, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		*plainText = string(b)
		return nil
	})
}

func (c *Client) do(ctx context.Context, method, endpoint string, body io.Reader, bodyDecoder func(reader io.Reader) error) (*int, error) {
	return c.doWithHeaders(ctx, method, endpoint, body, nil, bodyDecoder)
}

func (c *Client) doWithHeaders(ctx context.Context, method, endpoint string, body io.Reader, headers map[string]string, bodyDecoder func(reader io.Reader) error) (*int, error) {
	var reqBody []byte
	var err error

	debugEnabled := isDebugEnabled(c.config.Logger, ctx)
	if debugEnabled && body != nil {
		reqBody, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("reading request body for debug: %w", err)
		}
		body = bytes.NewReader(reqBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("%s%s", c.config.Host, endpoint), body)
	if err != nil {
		return nil, fmt.Errorf("error creating request `[%s] %s : %w`", method, fmt.Sprintf("%s%s", c.config.Host, endpoint), err)
	}
	req.Header.Add(headerAuthorization, fmt.Sprintf("Bearer %s", c.config.Token))
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	if debugEnabled {
		if len(reqBody) > 0 {
			c.config.Logger.Debug("request", "method", req.Method, "url", redactURL(req.URL.String()), "body", redactJSON(reqBody))
		} else {
			c.config.Logger.Debug("request", "method", req.Method, "url", redactURL(req.URL.String()))
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error in request `[%s] %s`: %w", method, fmt.Sprintf("%s%s", c.config.Host, endpoint), err)
	}
	defer resp.Body.Close()

	var respBody []byte
	if debugEnabled {
		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading response body for debug: %w", err)
		}
		c.config.Logger.Debug("response", "method", req.Method, "url", redactURL(req.URL.String()), "status", resp.StatusCode, "content-type", resp.Header.Get("Content-Type"), "body", redactJSON(respBody))
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if resp.StatusCode == http.StatusBadRequest {
		br := badRequestResponse{}
		if err := json.NewDecoder(resp.Body).Decode(&br); err == nil && br.Message != "" {
			return nil, errors.New(br.Message)
		}
		return nil, fmt.Errorf("bad request (%d)", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}

	if resp.StatusCode >= 300 {
		body := respBody
		if body == nil {
			body, _ = io.ReadAll(io.LimitReader(resp.Body, 1024))
		} else if len(body) > 1024 {
			body = body[:1024]
		}
		if len(body) > 0 {
			return nil, fmt.Errorf("wrong response code `%d`: %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("wrong response code `%d`", resp.StatusCode)
	}

	reader := resp.Body
	if err := bodyDecoder(reader); err != nil {
		return nil, fmt.Errorf("error decoding response body `[%s] %s: %w`", method, fmt.Sprintf("%s%s", c.config.Host, endpoint), err)
	}
	return &resp.StatusCode, nil
}

func (c *Client) doRequest(ctx context.Context, method, endpoint string, body io.Reader, bodyDecoder func(reader io.Reader) error) error {
	_, err := c.do(ctx, method, endpoint, body, bodyDecoder)
	return err
}

func (c *Client) doRequestWithHeaders(ctx context.Context, method, endpoint string, body io.Reader, headers map[string]string, bodyDecoder func(reader io.Reader) error) error {
	_, err := c.doWithHeaders(ctx, method, endpoint, body, headers, bodyDecoder)
	return err
}
