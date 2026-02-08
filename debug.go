package go_ha_client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
)

const debugBodyLimit = 2048

func isDebugEnabled(logger *slog.Logger, ctx context.Context) bool {
	if logger == nil {
		return false
	}
	return logger.Enabled(ctx, slog.LevelDebug)
}

// secretKeys are JSON keys that should be redacted in logs.
var secretKeys = []string{"access_token", "token", "password", "api_key", "secret"}

// redactJSON redacts sensitive keys from JSON body for logging.
func redactJSON(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// Not valid JSON, return truncated original
		return truncateForLog(body)
	}

	redactMap(data)
	redacted, err := json.Marshal(data)
	if err != nil {
		return truncateForLog(body)
	}
	return truncateForLog(redacted)
}

// redactMap recursively redacts sensitive keys in a map.
func redactMap(data map[string]interface{}) {
	for key, value := range data {
		for _, secret := range secretKeys {
			if key == secret {
				data[key] = "<redacted>"
				break
			}
		}
		// Recursively handle nested maps
		if nested, ok := value.(map[string]interface{}); ok {
			redactMap(nested)
		}
	}
}

// redactURL redacts sensitive query parameters from URL.
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	for _, key := range secretKeys {
		if q.Has(key) {
			q.Set(key, "<redacted>")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func truncateForLog(body []byte) string {
	if len(body) <= debugBodyLimit {
		return string(body)
	}
	return fmt.Sprintf("%s... (%d bytes truncated)", string(body[:debugBodyLimit]), len(body)-debugBodyLimit)
}

func formatWSLogPayload(payload interface{}) string {
	if payload == nil {
		return "<nil>"
	}
	if m, ok := payload.(map[string]interface{}); ok {
		logPayload := cloneWSRequest(m)
		if _, ok := logPayload["access_token"]; ok {
			logPayload["access_token"] = "<redacted>"
		}
		if _, ok := logPayload["token"]; ok {
			logPayload["token"] = "<redacted>"
		}
		if b, err := json.Marshal(logPayload); err == nil {
			return truncateForLog(b)
		}
	}
	if b, err := json.Marshal(payload); err == nil {
		return truncateForLog(b)
	}
	return fmt.Sprintf("%T", payload)
}
