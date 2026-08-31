package app

import (
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

func TestDurableJobDescriptorsOwnCanonicalPersistedWorkClass(t *testing.T) {
	server := newScannerTestServer(t)
	server.jobWakeForTests = func() {}
	seenTypes := map[string]bool{}
	for _, descriptor := range durableJobDescriptors {
		if seenTypes[descriptor.Type] {
			t.Fatalf("duplicate durable job descriptor for %q", descriptor.Type)
		}
		seenTypes[descriptor.Type] = true
		if !descriptor.WorkClass.Valid() {
			t.Fatalf("job %q has invalid work class %q", descriptor.Type, descriptor.WorkClass)
		}
		if descriptor.ResourceLane == "" {
			t.Fatalf("job %q has no physical resource lane", descriptor.Type)
		}
		job, err := server.createJobForWithMetadata(descriptor.Type, "Descriptor test.", "descriptor", descriptor.Type, nil)
		if err != nil {
			t.Fatalf("create %q: %v", descriptor.Type, err)
		}
		var persisted string
		if err := server.db.QueryRow(`SELECT priority FROM jobs WHERE id = ?`, job.ID).Scan(&persisted); err != nil {
			t.Fatalf("read %q priority: %v", descriptor.Type, err)
		}
		if persisted != string(descriptor.WorkClass) || job.Priority != descriptor.WorkClass {
			t.Fatalf("job %q class: envelope=%q persisted=%q descriptor=%q", descriptor.Type, job.Priority, persisted, descriptor.WorkClass)
		}
	}
	if len(seenTypes) == 0 || len(foundationcontract.CanonicalWorkClasses()) != 8 {
		t.Fatal("durable job or Foundation work vocabulary unexpectedly empty")
	}
}
