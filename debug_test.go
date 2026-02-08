package go_ha_client

import (
	"strings"
	"testing"
)

func TestRedactJSONRedactsNestedValues(t *testing.T) {
	input := []byte(`{"token":"token-value","nested":{"password":"password-value","arr":[{"api_key":"api-key-value"},{"value":"ok"}]},"list":["x",{"secret":"secret-value"}]}`)

	redacted := redactJSON(input)

	for _, value := range []string{"token-value", "password-value", "api-key-value", "secret-value"} {
		if strings.Contains(redacted, value) {
			t.Fatalf("expected redaction to remove %q from output", value)
		}
	}
	if !containsRedactedMarker(redacted) {
		t.Fatalf("expected redaction marker in output")
	}
}

func TestFormatWSLogPayloadRedactsNestedValues(t *testing.T) {
	payload := map[string]interface{}{
		"token": "token-value",
		"nested": map[string]interface{}{
			"access_token": "access-token-value",
		},
		"list": []interface{}{
			map[string]interface{}{
				"password": "password-value",
			},
		},
	}

	redacted := formatWSLogPayload(payload)

	for _, value := range []string{"token-value", "access-token-value", "password-value"} {
		if strings.Contains(redacted, value) {
			t.Fatalf("expected redaction to remove %q from output", value)
		}
	}
	if !containsRedactedMarker(redacted) {
		t.Fatalf("expected redaction marker in output")
	}
}

func containsRedactedMarker(value string) bool {
	return strings.Contains(value, "<redacted>") || strings.Contains(value, "\\u003credacted\\u003e")
}
