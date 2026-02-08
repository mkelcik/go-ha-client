package go_ha_client

import (
	"errors"
	"fmt"
	"strings"
)

// BuildEntityID constructs a Home Assistant entity_id from domain and objectID.
func BuildEntityID(domain, objectID string) string {
	if domain == "" || objectID == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s", domain, objectID)
}

// ParseEntityID splits entity_id into domain and objectID.
func ParseEntityID(entityID string) (string, string, error) {
	if strings.Count(entityID, ".") != 1 {
		return "", "", errors.New("invalid entity_id")
	}
	domain, objectID, ok := strings.Cut(entityID, ".")
	if !ok || domain == "" || objectID == "" {
		return "", "", errors.New("invalid entity_id")
	}
	return domain, objectID, nil
}
