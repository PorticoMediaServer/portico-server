package app

import (
	"reflect"
	"testing"
)

func TestPorticoLocalDiscoveryPublishesVersionedSinglePortContract(t *testing.T) {
	want := []string{
		"txtVersion=1",
		"path=/",
		"scheme=http",
		"serverId=srv_test",
		"fingerprint=sha256:test",
		"name=Test Server",
	}
	if got := porticoDiscoveryText(" srv_test ", " sha256:test ", " Test Server "); !reflect.DeepEqual(got, want) {
		t.Fatalf("discovery TXT = %#v, want %#v", got, want)
	}
}
