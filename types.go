package go_ha_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	Domain   string             `json:"domain"`
	Services map[string]Service `json:"services"`
}

type Service struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Fields      map[string]ServiceField `json:"fields"`
	Target      struct {
		Entity struct {
			Domain string `json:"domain"`
		} `json:"entity"`
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
	When     time.Time `json:"when"`
	Name     string    `json:"name"`
	State    string    `json:"state"`
	EntityId string    `json:"entity_id"`
	Icon     string    `json:"icon"`
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

type DefaultServiceCmd struct {
	Service  string `json:"-"`
	Domain   string `json:"-"`
	EntityId string `json:"entity_id"`
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
		return ""
	}
	return fmt.Sprintf("%s?%s", startTimeString, strings.Join(queryParams, "&"))
}

func createParamMap(filter interface{}) []string {
	queryParams := make([]string, 0, 10)
	v := reflect.ValueOf(filter).Elem()
	for i := 0; i < v.NumField(); i++ {
		paramName := v.Type().Field(i).Tag.Get("json")
		if paramName != "" && !v.Field(i).IsZero() {
			v := v.Field(i).Interface()
			if t, ok := v.(time.Time); ok {
				v = url.QueryEscape(t.Format(filterDateFormat))
			}

			queryParams = append(queryParams, fmt.Sprintf("%s=%s", paramName, v))
		}
	}
	return queryParams
}
