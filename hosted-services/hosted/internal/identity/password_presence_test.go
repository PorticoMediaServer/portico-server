package identity

import (
	"golang.org/x/crypto/bcrypt"
	"testing"
)

func TestPasswordPresenceNeverAuthorizesAbsentOrMalformedHash(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("CurrentPass9"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	// Making dummy work match the supplied password ensures its successful
	// comparison can never accidentally authorize an absent credential.
	s := &Service{dummy: hash}
	badSalt := append([]byte(nil), hash...)
	badSalt[7] = '!'
	badChecksum := append([]byte(nil), hash...)
	badChecksum[40] = '!'
	otherFormat := append([]byte(nil), hash...)
	otherFormat[2] = 'b'
	for _, missing := range [][]byte{nil, {}, []byte("invalid"), badSalt, badChecksum, otherFormat} {
		if PasswordInstalled(missing) || s.passwordMatches(missing, "CurrentPass9") {
			t.Fatal("absent credential authorized")
		}
	}
	if !PasswordInstalled(hash) || !s.passwordMatches(hash, "CurrentPass9") || s.passwordMatches(hash, "WrongPass9") {
		t.Fatal("current password verification failed")
	}
}
