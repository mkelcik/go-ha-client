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
	"io/ioutil"
	"net/http"
)


const (
	epPing                = "/api/"
	epConfig              = "/api/config"
	epDiscoveryInfo       = "/api/discovery_info"
	epEvents              = "/api/events"
	epServices            = "/api/services"
	epHistoryStateChanges = "/api/history/period"
	epLogbook             = "/api/logbook"
	epState               = "/api/states"
	epStateEntity         = "/api/states/%s"
	epPlainErrorLog       = "/api/error_log"
	epCameraProxy         = "/api/camera_proxy/%s"
	epCallService         = "/api/services/%s/%s"
)

var NotFoundError = errors.New("not found")
var UnAuthorizedError = errors.New("unauthorized")

type badRequestResponse struct {
	Message string `json:"message"`
}

type ClientConfig struct {
	Debug bool
	Token string
	Host  string
}

type Client struct {
	config     ClientConfig
	httpClient *http.Client
}

func NewClient(config ClientConfig, client *http.Client) *Client {
	return &Client{
		config:     config,
		httpClient: client,
	}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.doGetRequestJson(ctx, epPing, nil)
}

func (c *Client) GetConfig(ctx context.Context) (Config, error) {
	config := Config{}
	return config, c.doGetRequestJson(ctx, epConfig, &config)
}

func (c *Client) GetDiscoverInfo(ctx context.Context) (DiscoveryInfo, error) {
	discoverInfo := DiscoveryInfo{}
	return discoverInfo, c.doGetRequestJson(ctx, epDiscoveryInfo, &discoverInfo)
}

func (c *Client) GetEvents(ctx context.Context) (Events, error) {
	events := Events{}
	return events, c.doGetRequestJson(ctx, epEvents, &events)
}

func (c *Client) GetServices(ctx context.Context) (Services, error) {
	services := Services{}
	return services, c.doGetRequestJson(ctx, epServices, &services)
}

func (c *Client) GetStateChangesHistory(ctx context.Context, filter *StateChangesFilter) (StateChanges, error) {
	changes := StateChanges{}
	return changes, c.doGetRequestJson(ctx, epHistoryStateChanges+filter.String(), &changes)
}

func (c *Client) GetStates(ctx context.Context) (StateEntities, error) {
	states := StateEntities{}
	return states, c.doGetRequestJson(ctx, epState, &states)
}

func (c *Client) GetStateForEntity(ctx context.Context, entityId string) (StateEntity, error) {
	state := StateEntity{}
	if entityId == "" {
		return state, errors.New("wrong entityId")
	}
	return state, c.doGetRequestJson(ctx, fmt.Sprintf(epStateEntity, entityId), &state)
}

func (c *Client) GetLogbook(ctx context.Context, filter *LogbookFilter) (LogbookRecords, error) {
	logbookRecords := LogbookRecords{}
	return logbookRecords, c.doGetRequestJson(ctx, epLogbook+filter.String(), &logbookRecords)
}

func (c *Client) GetPlainErrorLog(ctx context.Context) (PlainText, error) {
	plaintText := ""
	return PlainText(plaintText), c.doGetRequestPlain(ctx, epPlainErrorLog, &plaintText)
}

func (c *Client) GetCameraJpeg(ctx context.Context, cameraEntityId string) (image.Image, error) {
	if cameraEntityId == "" {
		return nil, errors.New("wrong entityId")
	}
	var img image.Image
	return img, c.doRequest(ctx, http.MethodGet, fmt.Sprintf(epCameraProxy, cameraEntityId), nil, func(reader io.Reader) error {
		var err error
		img, err = jpeg.Decode(reader)
		if err != nil {
			return err
		}
		return nil
	})
}

func (c *Client) CallService(ctx context.Context, cmd DefaultServiceCmd) (StateEntities, error) {
	states := StateEntities{}

	if cmd.Service == "" {
		return states, errors.New("empty service name")
	}

	if cmd.Domain == "" {
		return states, errors.New("empty domain name")
	}

	return states, c.doPostRequestJson(ctx, fmt.Sprintf(epCallService, cmd.Domain, cmd.Service), cmd.Reader(), &states)
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
	return c.doRequest(ctx, http.MethodPost, endpoint, body, func(reader io.Reader) error {
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
		b, err := ioutil.ReadAll(reader)
		if err != nil {
			return err
		}
		*plainText = string(b)
		return nil
	})
}

func (c *Client) doRequest(ctx context.Context, method, endpoint string, body io.Reader, bodyDecoder func(reader io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("%s%s", c.config.Host, endpoint), body)
	if err != nil {
		return fmt.Errorf("error creating request `[%s] %s : %w`", method, fmt.Sprintf("%s%s", c.config.Host, endpoint), err)
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.config.Token))

	if c.config.Debug {
		fmt.Printf("[HA Client] [%s] `%s`\n", req.Method, req.URL.String())
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error in request `[%s] %s`", method, fmt.Sprintf("%s%s", c.config.Host, endpoint))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return NotFoundError
	}

	if resp.StatusCode == http.StatusBadRequest {
		br := badRequestResponse{}
		_ = json.NewDecoder(resp.Body).Decode(&br)
		return errors.New(br.Message)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return UnAuthorizedError
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wrong response code `%d`", resp.StatusCode)
	}

	var reader io.Reader
	reader = resp.Body

	// for debug purpose
	if c.config.Debug {
		body, _ := ioutil.ReadAll(resp.Body)
		reader = bytes.NewBuffer(body)
		fmt.Printf("[HA Client] [%s] `%s` response: %s \n", req.Method, req.URL.String(), string(body))
	}
	if err := bodyDecoder(reader); err != nil {
		return fmt.Errorf("error decoding request body `[%s] %s: %w`", method, fmt.Sprintf("%s%s", c.config.Host, endpoint), err)
	}
	return nil
}
