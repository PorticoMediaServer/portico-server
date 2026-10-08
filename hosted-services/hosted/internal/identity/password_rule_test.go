package identity

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// The only password rule is length: 8 or more characters counted as runes,
// within bcrypt's 72 bytes. No character classes are required.
func TestPasswordRuleIsLengthOnly(t *testing.T) {
	for password, want := range map[string]bool{
		"abcdefgh":              true,  // lowercase only
		"12345678":              true,  // digits only
		"        ":              true,  // spaces count
		"éééééééé":              true,  // 8 runes, 16 bytes
		"ééééééé":               false, // 7 runes, 14 bytes
		"abcdefg":               false,
		strings.Repeat("a", 72): true,
		strings.Repeat("a", 73): false,
		strings.Repeat("é", 36): true, // 72 bytes
		strings.Repeat("é", 37): false,
		string([]byte{0xff, 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'}): false,
	} {
		if got := ValidPassword(password); got != want {
			t.Errorf("ValidPassword(%q) = %v, want %v", password, got, want)
		}
	}
}

// Passwords are hashed at cost 10, and PasswordInstalled accepts exactly that
// cost so every account takes the same bcrypt time.
func TestPasswordCostIsTen(t *testing.T) {
	if PasswordCost != 10 {
		t.Fatalf("PasswordCost = %d", PasswordCost)
	}
	service := New(nil)
	hash, err := service.HashPassword(t.Context(), "lowercase only")
	if err != nil {
		t.Fatal(err)
	}
	if cost, _ := bcrypt.Cost(hash); cost != PasswordCost || !PasswordInstalled(hash) {
		t.Fatalf("hash cost %d installed=%v", cost, PasswordInstalled(hash))
	}
	if cost, _ := bcrypt.Cost(service.dummy); cost != PasswordCost {
		t.Fatalf("dummy cost %d", cost)
	}
}
