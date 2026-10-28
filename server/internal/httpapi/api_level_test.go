package httpapi

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// The web app and the server it opens must agree on the level they speak.
func TestAPILevelMatchesContracts(t *testing.T) {
	raw, err := os.ReadFile("../../../packages/contracts/src/api-level.ts")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`export const API_LEVEL = (\d+);`).FindSubmatch(raw)
	if match == nil {
		t.Fatal("packages/contracts/src/api-level.ts has no API_LEVEL")
	}
	if level, _ := strconv.Atoi(string(match[1])); level != APILevel {
		t.Fatalf("contracts API_LEVEL %d, server APILevel %d", level, APILevel)
	}
}
