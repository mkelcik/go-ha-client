package go_ha_client

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

const debugBodyLimit = 2048

type stdLogger struct {
	logger *log.Logger
}

func (l *stdLogger) Debugf(format string, args ...interface{}) {
	l.logger.Printf(format, args...)
}

func ensureLogger(config *ClientConfig) {
	if config.Logger != nil {
		return
	}
	config.Logger = &stdLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
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
