package dns01

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestRefusesUnscopedOrPublicUpdates(t *testing.T) {
	id := "srv_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	value := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	for _, c := range []Client{{Address: "1.1.1.1:53", Zone: "acme.getportico.tv"}, {Address: "10.77.0.2:5354", Zone: "getportico.tv"}, {Address: "dns.internal:5354", Zone: "acme.getportico.tv"}} {
		if c.Present(context.Background(), id, value) == nil {
			t.Fatal("unsafe update accepted")
		}
	}
}
