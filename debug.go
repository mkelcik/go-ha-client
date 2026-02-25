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

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// Not valid JSON, return truncated original
		return truncateForLog(body)
	}

	redacted := redactValue(data)
	out, err := json.Marshal(redacted)
	if err != nil {
		return truncateForLog(body)
	}
	return truncateForLog(out)
}

func redactValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, val := range v {
			if isSecretKey(key) {
				v[key] = "<redacted>"
				continue
			}
			v[key] = redactValue(val)
		}
		return v
	case []interface{}:
		for i := range v {
			v[i] = redactValue(v[i])
		}
		return v
	default:
		return value
	}
}

func isSecretKey(key string) bool {
	for _, secret := range secretKeys {
		if key == secret {
			return true
		}
	}
	return false
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
	if b, err := json.Marshal(payload); err == nil {
		return redactJSON(b)
	}
	return fmt.Sprintf("%T", payload)
}
