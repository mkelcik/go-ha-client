package go_ha_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
)

const (
	filterDateFormat = "2006-01-02T15:04:05-07:00"
)

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

type DiscoveryInfo struct {
	BaseUrl             string `json:"base_url"`
	LocationName        string `json:"location_name"`
	RequiresApiPassword bool   `json:"requires_api_password"`
	Version             string `json:"version"`
}

type Events []Event

type Event struct {
	Event         string `json:"event"`
	ListenerCount int    `json:"listener_count"`
}

type Services []ServiceDomain

type ServiceDomain struct {
	Domain   string     `json:"domain"`
	Services ServiceMap `json:"services"`
}

type ServiceMap map[string]Service

func (m *ServiceMap) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}

	switch b[0] {
	case '{':
		var services map[string]Service
		if err := json.Unmarshal(b, &services); err != nil {
			return err
		}
		*m = services
		return nil
	case '[':
		var list []string
		if err := json.Unmarshal(b, &list); err != nil {
			return err
		}
		services := make(map[string]Service, len(list))
		for _, name := range list {
			services[name] = Service{Name: name}
		}
		*m = services
		return nil
	default:
		return fmt.Errorf("unexpected services JSON: %s", string(b))
	}
}

type Service struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Fields      map[string]ServiceField `json:"fields"`
	Target      struct {
		Entity []map[string]interface{} `json:"entity"`
	} `json:"target"`
}

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
	FilterEntityId         string    `json:"filter_entity_id"`
	MinimalResponse        bool      `json:"minimal_response"`
	NoAttributes           bool      `json:"no_attributes"`
	SignificantChangesOnly bool      `json:"significant_changes_only"`
}

func (f *StateChangesFilter) String() string {
	return createQueryString(f.StartTime, f)
}

type StateChanges [][]EntityChange

type EntityChange struct {
	EntityId    string                 `json:"entity_id"`
	State       string                 `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
	LastChanged time.Time              `json:"last_changed"`
	LastUpdated time.Time              `json:"last_updated"`
}

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

type LogbookRecords []LogbookRecord

type LogbookRecord struct {
	When          time.Time `json:"when"`
	Name          string    `json:"name"`
	Message       string    `json:"message"`
	Domain        string    `json:"domain"`
	EntityId      string    `json:"entity_id"`
	ContextUserId *string   `json:"context_user_id"`
	State         string    `json:"state"`
	Icon          string    `json:"icon"`
}

type LogbookFilter struct {
	StartTime time.Time
	EndTime   time.Time `json:"end_time"`
	EntityId  string    `json:"entity"`
}

func (f *LogbookFilter) String() string {
	return createQueryString(f.StartTime, f)
}

type StateEntities []StateEntity

type Calendars []Calendar

type Calendar struct {
	Name     string `json:"name"`
	EntityId string `json:"entity_id"`
}

type CalendarEvents []CalendarEvent

type CalendarEvent struct {
	Summary     string            `json:"summary"`
	Description string            `json:"description,omitempty"`
	Location    string            `json:"location,omitempty"`
	UID         string            `json:"uid,omitempty"`
	Start       CalendarEventTime `json:"start"`
	End         CalendarEventTime `json:"end"`
}

type CalendarEventTime struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type StateEntity struct {
	EntityId    string                 `json:"entity_id"`
	State       string                 `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
	LastChanged time.Time              `json:"last_changed"`
	LastUpdated time.Time              `json:"last_updated"`
	Context     struct {
		Id       string `json:"id"`
		ParentId string `json:"parent_id"`
		UserId   string `json:"user_id"`
	} `json:"context"`
}

type PlainText string

// NewTurnLightOnCmd is helper for turning light on
func NewTurnLightOnCmd(entityId string) DefaultServiceCmd {
	return DefaultServiceCmd{
		Service:  "turn_on",
		Domain:   "light",
		EntityId: entityId,
	}
}

// NewTurnLightOffCmd is helper for turning light off
func NewTurnLightOffCmd(entityId string) DefaultServiceCmd {
	return DefaultServiceCmd{
		Service:  "turn_off",
		Domain:   "light",
		EntityId: entityId,
	}
}

// NewToggleLightTCmd is helper for turning light off
func NewToggleLightTCmd(entityId string) DefaultServiceCmd {
	return DefaultServiceCmd{
		Service:  "toggle",
		Domain:   "light",
		EntityId: entityId,
	}
}

type DefaultServiceCmd struct {
	Service  string `json:"-"`
	Domain   string `json:"-"`
	EntityId string `json:"entity_id"`
}

type newEventRising struct {
	NextRising *time.Time `json:"next_rising,omitempty"`
}

type templateRequest struct {
	Template string `json:"template"`
}

type ConfigurationCheckResult struct {
	Errors *string `json:"errors"`
	Result string  `json:"result"`
}

type IntentRequest struct {
	Name string                 `json:"name"`
	Data map[string]interface{} `json:"data,omitempty"`
}

type IntentResponse struct {
	Response map[string]interface{} `json:"response"`
}

type ConversationProcessRequest struct {
	Text           string `json:"text"`
	Language       string `json:"language,omitempty"`
	AgentId        string `json:"agent_id,omitempty"`
	ConversationId string `json:"conversation_id,omitempty"`
}

type ConversationProcessResponse struct {
	ContinueConversation bool                 `json:"continue_conversation,omitempty"`
	ConversationId       string               `json:"conversation_id,omitempty"`
	Response             ConversationResponse `json:"response"`
}

type ConversationResponse struct {
	ResponseType string                 `json:"response_type,omitempty"`
	Language     string                 `json:"language,omitempty"`
	Data         map[string]interface{} `json:"data,omitempty"`
	Speech       map[string]interface{} `json:"speech,omitempty"`
}

type ServiceCallResponse struct {
	ChangedStates   StateEntities              `json:"changed_states"`
	StateChanges    StateEntities              `json:"state_changes,omitempty"`
	ServiceResponse map[string]json.RawMessage `json:"service_response"`
}

type WeatherForecastRequest struct {
	EntityId string `json:"entity_id"`
	Type     string `json:"type,omitempty"`
}

type WeatherForecasts struct {
	Forecast []WeatherForecast `json:"forecast"`
}

type WeatherForecast map[string]interface{}

type StateResponse struct {
	State
	CreateCode  int       `json:"-"`
	EntityId    string    `json:"entity_id"`
	LastChanged time.Time `json:"last_changed"`
	LastUpdated time.Time `json:"last_updated"`
}

func (s StateResponse) Created() bool {
	return s.CreateCode == http.StatusCreated
}

func (s StateResponse) Updated() bool {
	return s.CreateCode == http.StatusOK
}

type State struct {
	State      string                 `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

func (c DefaultServiceCmd) Reader() io.Reader {
	b, _ := json.Marshal(c)
	return bytes.NewBuffer(b)
}

func createQueryString(startTime time.Time, filter interface{}) string {
	if filter == nil {
		return ""
	}

	// hack because start time is different (https://developers.home-assistant.io/docs/api/rest)
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
	queryParams := make([]string, 0, 10)
	v := reflect.ValueOf(filter).Elem()
	for i := 0; i < v.NumField(); i++ {
		paramName := v.Type().Field(i).Tag.Get("json")
		if paramName != "" && !v.Field(i).IsZero() {
			fieldValue := v.Field(i).Interface()
			value := fmt.Sprint(fieldValue)
			if t, ok := fieldValue.(time.Time); ok {
				value = t.Format(filterDateFormat)
			}

			queryParams = append(queryParams, fmt.Sprintf("%s=%s", paramName, url.QueryEscape(value)))
		}
	}
	return queryParams
}
