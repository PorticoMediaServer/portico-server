package app

import (
	"reflect"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

func TestServerGrantablePermissionCatalogComesFromFoundationContract(t *testing.T) {
	if !reflect.DeepEqual(permissionCatalog(), foundationcontract.GrantablePermissionIDs()) {
		t.Fatal("server grantable permissions drifted from the generated Foundation contract")
	}
	grantable := make(map[string]bool)
	for _, permission := range permissionCatalog() {
		grantable[permission] = true
	}
	for _, ownerOnly := range []string{"manageLibraries", "manageUsers", "manageServer"} {
		if grantable[ownerOnly] {
			t.Fatalf("owner-only permission %q became delegable", ownerOnly)
		}
	}
}

func TestServerWorkClassesAreExactlyTheGeneratedFoundationVocabulary(t *testing.T) {
	classes := foundationcontract.CanonicalWorkClasses()
	generated := foundationcontract.WorkClasses()
	if len(classes) != len(generated) {
		t.Fatalf("typed work classes=%d, generated=%d", len(classes), len(generated))
	}
	for index, class := range classes {
		if string(class) != generated[index] {
			t.Fatalf("typed work class %d=%q, generated=%q", index, class, generated[index])
		}
		if class.Priority() != index+1 {
			t.Fatalf("work class %q priority=%d, expected %d", class, class.Priority(), index+1)
		}
	}
}
