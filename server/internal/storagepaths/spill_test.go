package storagepaths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryTranscodesRejectDiskAndSpillWithinCap(t *testing.T) {
	disk := filepath.Join(t.TempDir(), "disk")
	if _, err := NewTranscodePool(t.TempDir(), disk, 100, []Mount{{"/", "ext4"}}); err == nil {
		t.Fatal("ordinary disk advertised as RAM")
	}
	info, err := os.Stat("/dev/shm")
	if err != nil || !info.IsDir() {
		t.Skip("RAM filesystem unavailable")
	}
	mountFile, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		t.Skip(err)
	}
	mounts := ParseMountInfo(mountFile)
	mountFile.Close()
	if Filesystem("/dev/shm", mounts) != "tmpfs" {
		t.Skip("/dev/shm is not tmpfs")
	}
	memory, err := os.MkdirTemp("/dev/shm", "portico-spill-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(memory)
	pool, err := NewTranscodePool(memory, disk, 32, mounts)
	if err != nil {
		t.Fatal(err)
	}
	a, err := pool.Create()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Remove()
	if _, err = a.Write([]byte("123456")); err != nil {
		t.Fatal(err)
	}
	b, err := pool.Create()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Remove()
	if _, err = b.Write([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(a.Path()) != memory || filepath.Dir(b.Path()) != disk || pool.used > 8 {
		t.Fatal("aggregate cap exceeded", a.Path(), b.Path(), pool.used)
	}
	if _, err = a.Write([]byte("789")); err != nil {
		t.Fatal(err)
	}
	a.Close()
	if filepath.Dir(a.Path()) != disk {
		t.Fatal("did not spill")
	}
	raw, err := os.ReadFile(a.Path())
	if err != nil || string(raw) != "123456789" {
		t.Fatal(string(raw), err)
	}
	if entries, _ := os.ReadDir(memory); len(entries) != 0 {
		t.Fatal("RAM bytes remained after spill")
	}
	// A full bounded pool routes subsequent output directly to disk.
	c, err := pool.Create()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Remove()
	c.Write([]byte(strings.Repeat("x", 9)))
	if filepath.Dir(c.Path()) != disk {
		t.Fatal("over-cap file retained in RAM")
	}
}
