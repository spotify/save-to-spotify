package cmd

import (
	"regexp"
	"testing"
)

func TestGenerateUUID(t *testing.T) {
	uuid := generateUUID()

	// UUID v4 format: 8-4-4-4-12 hex chars
	pattern := `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	matched, err := regexp.MatchString(pattern, uuid)
	if err != nil {
		t.Fatalf("regex error: %v", err)
	}
	if !matched {
		t.Errorf("UUID %q does not match v4 format", uuid)
	}
}

func TestGenerateUUID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		uuid := generateUUID()
		if seen[uuid] {
			t.Fatalf("duplicate UUID: %s", uuid)
		}
		seen[uuid] = true
	}
}
