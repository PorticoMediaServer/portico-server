package app

import "testing"

func TestPorticoInviteeProvisioningFromIntrospectionMember(t *testing.T) {
	server := newScannerTestServer(t)
	now := "2026-05-03T23:23:35Z"
	libraryIDs := []string{"lib_debug_movies", "lib_debug_tv"}
	for _, id := range libraryIDs {
		if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, sort_order, created_at) VALUES (?, ?, 'movie', 0, ?)`, id, id, now); err != nil {
			t.Fatalf("seed library %s: %v", id, err)
		}
	}
	user, err := server.userForPorticoMembership(RemoteAccessMember{
		ID:          "mem_debug_invitee",
		UserID:      "usr_debug_invitee",
		Email:       "invitee@example.test",
		DisplayName: "Invitee",
		Role:        "user",
		Status:      "active",
		PermissionTemplate: RemotePermissionTemplate{
			LibraryIDs: libraryIDs,
			Permissions: map[string]bool{
				"playMedia": true,
				"transcode": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("provision Portico invitee: %v", err)
	}
	if user.PorticoMembershipID != "mem_debug_invitee" || user.PorticoUserID != "usr_debug_invitee" || user.AuthOrigin != "portico" {
		t.Fatalf("unexpected provisioned user: %#v", user)
	}
}
