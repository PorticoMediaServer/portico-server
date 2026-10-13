package storage

import (
	"strconv"
	"strings"
)

// A Unix object identity is "dev:<st_dev>:ino:<st_ino>". The inode number
// belongs to the file; the device number belongs to this boot: network, FUSE
// and pooled mounts, btrfs subvolumes, ZFS datasets and removable disks get
// whatever number the kernel hands out in mount or detection order, so the
// same folder can come back under another one after a restart. Anything kept
// across restarts therefore compares the inode and treats a device number as
// evidence only within one observation.

// unixIdentity splits "dev:D:ino:N"; ok is false for every other form
// (a Windows volume and file id, a remote object id, an empty identity).
func unixIdentity(identity string) (device, inode uint64, ok bool) {
	rest, found := strings.CutPrefix(identity, "dev:")
	if !found {
		return 0, 0, false
	}
	d, i, found := strings.Cut(rest, ":ino:")
	if !found {
		return 0, 0, false
	}
	device, err := strconv.ParseUint(d, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	inode, err = strconv.ParseUint(i, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return device, inode, true
}

// SameRoot reports whether a recorded root identity and a freshly observed one
// name the same folder. Two Unix identities with the same inode are the same
// folder whatever their device numbers; every other form must match exactly.
// A different inode is still a different folder: an unmounted mount point is
// its own directory on the parent filesystem.
func SameRoot(recorded, observed string) bool {
	if recorded == observed {
		return recorded != ""
	}
	_, a, okA := unixIdentity(recorded)
	_, b, okB := unixIdentity(observed)
	return okA && okB && a == b
}

// StableObjectIdentity is the identity of an object under a root in the form
// that is kept across restarts. An object on the root's own filesystem is named
// by its inode alone ("ino:N"); one on another filesystem mounted beneath the
// root keeps its device number, because inode numbers repeat across
// filesystems. Identities of any other form are returned unchanged.
func StableObjectIdentity(object, root string) string {
	device, inode, ok := unixIdentity(object)
	if !ok {
		return object
	}
	rootDevice, _, ok := unixIdentity(root)
	if !ok || rootDevice != device {
		return object
	}
	return "ino:" + strconv.FormatUint(inode, 10)
}
