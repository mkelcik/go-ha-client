package go_ha_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	filterDateFormat = "2006-01-02T15:04:05-07:00"
)

// Common Home Assistant domain names for service calls.
const (
	DomainLight   = "light"
	DomainSwitch  = "switch"
	DomainScript  = "script"
	DomainScene   = "scene"
	DomainWeather = "weather"
)

// Common Home Assistant service names for service calls.
const (
	ServiceTurnOn      = "turn_on"
	ServiceTurnOff     = "turn_off"
	ServiceToggle      = "toggle"
	ServiceGetForecast = "get_forecasts"
)

// Common Home Assistant event types.
const (
	EventTypeStateChanged       = "state_changed"
	EventTypeCallService        = "call_service"
	EventTypeHomeAssistantStart = "homeassistant_start"
	EventTypeHomeAssistantStop  = "homeassistant_stop"
)

// Config represents the Home Assistant configuration.
type Config struct {
	Components   []string `json:"components"`
	ConfigDir    string   `json:"config_dir"`
	Elevation    int      `json:"elevation"`
	Latitude     float64  `json:"latitude"`
	LocationName string   `json:"location_name"`
	Longitude    float64  `json:"longitude"`
	TimeZone     string   `json:"time_zone"`
	UnitSystem   struct {
		Length      string `json:"length"`
		Mass        string `json:"mass"`
		Temperature string `json:"temperature"`
		Volume      string `json:"volume"`
	} `json:"unit_system"`
	Version               string   `json:"version"`
	WhitelistExternalDirs []string `json:"whitelist_external_dirs"`
	AllowlistExternalDirs []string `json:"allowlist_external_dirs"`
}

// Events is a list of events.
type Events []Event

// Event represents a single event listener count.
type Event struct {
	Event         string `json:"event"`
	ListenerCount int    `json:"listener_count"`
}

// Services is a list of service domains.
type Services []ServiceDomain

// ServiceDomain represents a service domain and its services.
type ServiceDomain struct {
	Domain   string     `json:"domain"`
	Services ServiceMap `json:"services"`
}

// ServiceMap is a map of services.
type ServiceMap map[string]Service

func (m *ServiceMap) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		*m = nil
		return nil
	}

	switch trimmed[0] {
	case '{':
		var services map[string]Service
		if err := json.Unmarshal(trimmed, &services); err != nil {
			return err
		}
		*m = services
		return nil
	case '[':
		var list []string
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return err
		}
		services := make(map[string]Service, len(list))
		for _, name := range list {
			services[name] = Service{Name: name}
		}
		*m = services
		return nil
	default:
		return fmt.Errorf("unexpected services JSON: %s", string(trimmed))
	}
}

// Service represents a service definition.
type Service struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Fields      map[string]ServiceField `json:"fields"`
	Target      struct {
		Entity []map[string]interface{} `json:"entity"`
	} `json:"target"`
}

// ServiceField represents a field in a service definition.
type ServiceField struct {
	Advanced    bool                              `json:"advanced"`
	Name        string                            `json:"name"`
	Description string                            `json:"description"`
	Required    bool                              `json:"required"`
	Example     interface{}                       `json:"example"`
	Selector    map[string]map[string]interface{} `json:"selector"`
}

// StateChangesFilter use json tags to construct queryParams
type StateChangesFilter struct {
	StartTime              time.Time
	EndTime                time.Time `json:"end_time"`
	FilterEntityID         string    `json:"filter_entity_id"`
	MinimalResponse        bool      `json:"minimal_response"`
	NoAttributes           bool      `json:"no_attributes"`
	SignificantChangesOnly bool      `json:"significant_changes_only"`
}

func (f *StateChangesFilter) String() string {
	return createQueryString(f.StartTime, f)
}

// StateChanges is a list of state changes for entities.
type StateChanges [][]EntityChange

// EntityChange represents a change in an entity's state.
type EntityChange struct {
	EntityID    string                 `json:"entity_id"`
	State       string                 `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
	LastChanged time.Time              `json:"last_changed"`
	LastUpdated time.Time              `json:"last_updated"`
}

// GetFriendlyName returns the friendly name of the entity if available.
func (e *EntityChange) GetFriendlyName() string {
	v, ok := e.Attributes["friendly_name"]
	if !ok {
		return ""
	}

	name, ok := v.(string)
	if !ok {
		return ""
	}

	return name
}

// LogbookRecords is a list of logbook records.
type LogbookRecords []LogbookRecord

// LogbookRecord represents a single logbook entry.
type LogbookRecord struct {
	When          time.Time `json:"when"`
	Name          string    `json:"name"`
	Message       string    `json:"message"`
	Domain        string    `json:"domain"`
	EntityID      string    `json:"entity_id"`
	ContextUserID *string   `json:"context_user_id"`
	State         string    `json:"state"`
	Icon          string    `json:"icon"`
}

// LogbookFilter defines filter for logbook queries.
type LogbookFilter struct {
	StartTime time.Time
	EndTime   time.Time `json:"end_time"`
	EntityID  string    `json:"entity"`
}

func (f *LogbookFilter) String() string {
	return createQueryString(f.StartTime, f)
}

// StateEntities is a list of state entities.
type StateEntities []StateEntity

// Calendars is a list of calendars.
type Calendars []Calendar

// Calendar represents a calendar entity.
type Calendar struct {
	Name     string `json:"name"`
	EntityID string `json:"entity_id"`
}

// CalendarEvents is a list of calendar events.
type CalendarEvents []CalendarEvent

// CalendarEvent represents a single calendar event.
type CalendarEvent struct {
	Summary     string            `json:"summary"`
	Description string            `json:"description,omitempty"`
	Location    string            `json:"location,omitempty"`
	UID         string            `json:"uid,omitempty"`
	Start       CalendarEventTime `json:"start"`
	End         CalendarEventTime `json:"end"`
}

// CalendarEventTime represents the time of a calendar event.
type CalendarEventTime struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

// StateEntity represents the state of an entity.
type StateEntity struct {
	EntityID    string                 `json:"entity_id"`
	State       string                 `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
	LastChanged time.Time              `json:"last_changed"`
	LastUpdated time.Time              `json:"last_updated"`
	Context     struct {
		ID       string `json:"id"`
		ParentID string `json:"parent_id"`
		UserID   string `json:"user_id"`
	} `json:"context"`
}

// PlainText represents plain text response.
type PlainText string

// NewTurnLightOnCmd is helper for turning light on
func NewTurnLightOnCmd(entityID string) DefaultServiceCmd {
	return DefaultServiceCmd{
		Service:  ServiceTurnOn,
		Domain:   DomainLight,
		EntityID: entityID,
	}
}

// NewTurnLightOffCmd is helper for turning light off
func NewTurnLightOffCmd(entityID string) DefaultServiceCmd {
	return DefaultServiceCmd{
		Service:  ServiceTurnOff,
		Domain:   DomainLight,
		EntityID: entityID,
	}
}

// NewToggleLightCmd is helper for toggling light state.
func NewToggleLightCmd(entityID string) DefaultServiceCmd {
	return DefaultServiceCmd{
		Service:  ServiceToggle,
		Domain:   DomainLight,
		EntityID: entityID,
	}
}

// NewServiceDataEntityID builds service_data payload with a single entity_id.
func NewServiceDataEntityID(entityID string) map[string]interface{} {
	return map[string]interface{}{
		"entity_id": entityID,
	}
}

// DefaultServiceCmd is a default implementation of a service call command.
type DefaultServiceCmd struct {
	Service  string `json:"-"`
	Domain   string `json:"-"`
	EntityID string `json:"entity_id"`
}

type newEventRising struct {
	NextRising *time.Time `json:"next_rising,omitempty"`
}

type templateRequest struct {
	Template string `json:"template"`
}

// ConfigurationCheckResult represents the result of a configuration check.
type ConfigurationCheckResult struct {
	Errors *string `json:"errors"`
	Result string  `json:"result"`
}

// IntentRequest represents a request to handle an intent.
type IntentRequest struct {
	Name string                 `json:"name"`
	Data map[string]interface{} `json:"data,omitempty"`
}

// IntentResponse represents the response from intent handling.
type IntentResponse struct {
	Response map[string]interface{} `json:"response"`
}

// ServiceCallResponse represents the response from a service call when return_response is requested.
type ServiceCallResponse struct {
	ChangedStates   StateEntities              `json:"changed_states"`
	StateChanges    StateEntities              `json:"state_changes,omitempty"`
	ServiceResponse map[string]json.RawMessage `json:"service_response"`
}

// WeatherForecastRequest represents a request for weather forecast.
type WeatherForecastRequest struct {
	EntityID string `json:"entity_id"`
	Type     string `json:"type,omitempty"`
}

// WeatherForecasts represents a list of weather forecasts.
type WeatherForecasts struct {
	Forecast []WeatherForecast `json:"forecast"`
}

// WeatherForecast represents a single weather forecast entry.
type WeatherForecast map[string]interface{}

// StateResponse represents the response from creating or updating a state.
type StateResponse struct {
	State
	CreateCode  int       `json:"-"`
	EntityID    string    `json:"entity_id"`
	LastChanged time.Time `json:"last_changed"`
	LastUpdated time.Time `json:"last_updated"`
}

// Created returns true if the state was created.
func (s StateResponse) Created() bool {
	return s.CreateCode == http.StatusCreated
}

// Updated returns true if the state was updated.
func (s StateResponse) Updated() bool {
	return s.CreateCode == http.StatusOK
}

// State represents the core state data.
type State struct {
	State      string                 `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

// Reader returns an io.Reader for the service command body.
func (c DefaultServiceCmd) Reader() io.Reader {
	b, _ := json.Marshal(c)
	return bytes.NewBuffer(b)
}

func createQueryString(startTime time.Time, filter interface{}) string {
	if filter == nil {
		return ""
	}

	// Home Assistant expects start time as a path segment on the history/logbook endpoints.
	startTimeString := ""
	if !startTime.IsZero() {
		startTimeString = fmt.Sprintf("/%s", startTime.Format(filterDateFormat))
	}

	queryParams := createParamMap(filter)

	if len(queryParams) == 0 {
		return startTimeString
	}
	return fmt.Sprintf("%s?%s", startTimeString, strings.Join(queryParams, "&"))
}

func createParamMap(filter interface{}) []string {
	switch f := filter.(type) {
	case *StateChangesFilter:
		return createStateChangesParams(f)
	case *LogbookFilter:
		return createLogbookParams(f)
	default:
		return nil
	}
}

func createStateChangesParams(filter *StateChangesFilter) []string {
	if filter == nil {
		return nil
	}

	queryParams := make([]string, 0, 5)
	if !filter.EndTime.IsZero() {
		queryParams = append(queryParams, fmt.Sprintf("end_time=%s", url.QueryEscape(filter.EndTime.Format(filterDateFormat))))
	}
	if filter.FilterEntityID != "" {
		queryParams = append(queryParams, fmt.Sprintf("filter_entity_id=%s", url.QueryEscape(filter.FilterEntityID)))
	}
	if filter.MinimalResponse {
		queryParams = append(queryParams, "minimal_response=true")
	}
	if filter.NoAttributes {
		queryParams = append(queryParams, "no_attributes=true")
	}
	if filter.SignificantChangesOnly {
		queryParams = append(queryParams, "significant_changes_only=true")
	}
	return queryParams
}

func createLogbookParams(filter *LogbookFilter) []string {
	if filter == nil {
		return nil
	}

	queryParams := make([]string, 0, 2)
	if !filter.EndTime.IsZero() {
		queryParams = append(queryParams, fmt.Sprintf("end_time=%s", url.QueryEscape(filter.EndTime.Format(filterDateFormat))))
	}
	if filter.EntityID != "" {
		queryParams = append(queryParams, fmt.Sprintf("entity=%s", url.QueryEscape(filter.EntityID)))
	}
	return queryParams
}
