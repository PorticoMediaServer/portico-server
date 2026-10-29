package main

// pendingTaggedUnions lists routes whose 2xx response unions still lack a
// required const discriminator (O-159 R-09b, P2-75-R09B-PLAN.md). apigen fails
// on any other undiscriminated response union. Each conversion removes its
// routes; the list ends empty in this batch.
var pendingTaggedUnions = map[string]bool{
	"get_file_info_query":                        true,
	"get_items_id":                               true,
	"get_items_id_detail":                        true,
	"get_reading_sessions_sessionId_annotations": true,
	"get_saved_content":                          true,
	"post_access_invitations_accept":             true,
	"post_auth_owner_reset_verify":               true,
	"post_setup_resume":                          true,
	"setup_server":                               true,
	"start_playback_session":                     true,
}
