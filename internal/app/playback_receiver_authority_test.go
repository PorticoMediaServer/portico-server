package app

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func receiverPublicKeyForTest(t *testing.T) string {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
}

func TestPlaybackReceiverKeyBoundAuthorizationLifecycle(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	seedExactPlaybackFactsForFixture(t, server, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	receiverKey := receiverPublicKeyForTest(t)
	var receiver PlaybackReceiver
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers", PlaybackReceiverRequest{
		ReceiverID: "receiver-living-room", Name: "Living Room", App: "Portico",
		Platform: "Android TV", ReceiverPublicKey: receiverKey,
		ClientInstanceID:  "receiver-client-living-room",
		SupportedCommands: []string{"stop", "load", "play", "pause", "seek", "load", "unsupported"},
	}, &receiver)
	if status != http.StatusCreated {
		t.Fatalf("register receiver status=%d body=%s", status, body)
	}
	if receiver.ID != "receiver-living-room" || receiver.ServerID == "" || receiver.ReceiverPublicKey != receiverKey || receiver.ReceiverPublicKeyFingerprint == "" {
		t.Fatalf("unexpected receiver: %#v", receiver)
	}
	if strings.Join(receiver.SupportedCommands, ",") != "load,pause,play,seek,stop" {
		t.Fatalf("receiver capabilities=%v", receiver.SupportedCommands)
	}

	var receivers ListResponse[PlaybackReceiver]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/receivers", nil, &receivers)
	if status != http.StatusOK || len(receivers.Items) != 1 || receivers.Items[0].ReceiverPublicKeyFingerprint != receiver.ReceiverPublicKeyFingerprint {
		t.Fatalf("list receivers status=%d body=%s receivers=%#v", status, body, receivers)
	}

	controllerKey := receiverPublicKeyForTest(t)
	authorize := ReceiverAuthorizationRequest{
		RequestID: "authorize-living-room-1", ControllerID: "controller-phone-1",
		ControllerPublicKey: controllerKey, AllowedCommands: []string{"load", "play", "pause", "seek", "stop"},
		ClientInstanceID: "controller-client-phone-1",
	}
	var grant ReceiverControllerGrant
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/authorizations", authorize, &grant)
	if status != http.StatusCreated {
		t.Fatalf("authorize receiver status=%d body=%s", status, body)
	}
	if grant.AuthorizationID == "" || grant.ReceiverID != receiver.ID || grant.ServerID != receiver.ServerID || grant.ReceiverPublicKeyFingerprint != receiver.ReceiverPublicKeyFingerprint || grant.AuthorizationRevision == "" {
		t.Fatalf("unexpected controller grant: %#v", grant)
	}
	if strings.Join(grant.AllowedCommands, ",") != "load,pause,play,seek,stop" {
		t.Fatalf("controller grant commands=%v", grant.AllowedCommands)
	}
	var replay ReceiverControllerGrant
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/authorizations", authorize, &replay)
	if status != http.StatusCreated || replay.AuthorizationID != grant.AuthorizationID {
		t.Fatalf("exact authorization retry status=%d body=%s grant=%#v", status, body, replay)
	}
	conflict := authorize
	conflict.AllowedCommands = []string{"load"}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/authorizations", conflict, nil)
	if status != http.StatusConflict {
		t.Fatalf("conflicting authorization retry status=%d body=%s", status, body)
	}

	var heartbeat PlaybackReceiverHeartbeatResponse
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback/receivers/"+receiver.ID, PlaybackReceiverHeartbeatRequest{ReceiverPublicKeyFingerprint: receiver.ReceiverPublicKeyFingerprint}, &heartbeat)
	if status != http.StatusOK || len(heartbeat.Authorizations) != 1 {
		t.Fatalf("receiver heartbeat status=%d body=%s heartbeat=%#v", status, body, heartbeat)
	}
	record := heartbeat.Authorizations[0]
	if record.AuthorizationID != grant.AuthorizationID || record.ControllerID != authorize.ControllerID || record.ControllerPublicKey != controllerKey || record.AuthorizationRevision != grant.AuthorizationRevision {
		t.Fatalf("receiver authorization did not match controller grant: %#v", record)
	}

	var source PlaybackResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", PlaybackSessionCreateRequest{
		MediaID: "movie_meridian", ClientInstanceID: authorize.ClientInstanceID,
		ClientProfile: authenticatedPlaybackRuntimeProfile(), Intent: automaticPlaybackIntent(), SkipPreroll: true,
	}, &source)
	if status != http.StatusOK {
		t.Fatalf("start controller playback status=%d body=%s", status, body)
	}
	forward := []playbackQueueOccurrence{{EntryID: "duplicate-forward-1", MediaID: source.Media.ID}, {EntryID: "duplicate-forward-2", MediaID: source.Media.ID}}
	history := []playbackQueueOccurrence{{HistoryID: "duplicate-history-1", EntryID: "duplicate-past-1", MediaID: source.Media.ID}, {HistoryID: "duplicate-history-2", EntryID: "duplicate-past-2", MediaID: source.Media.ID}}
	if err := server.replacePlaybackSessionQueue(t.Context(), source.SessionID, forward); err != nil {
		t.Fatal(err)
	}
	if err := server.replacePlaybackSessionHistory(t.Context(), source.SessionID, history); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT queue_revision FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&source.QueueRevision); err != nil {
		t.Fatal(err)
	}
	var handedOff PlaybackResponse
	handoffRequest := PlaybackReceiverHandoffRequest{
		AuthorizationID: grant.AuthorizationID, ReceiverPublicKeyFingerprint: receiver.ReceiverPublicKeyFingerprint,
		Playback: PlaybackSessionCreateRequest{
			MediaID: source.Media.ID, ClientProfile: authenticatedPlaybackRuntimeProfile(), Intent: automaticPlaybackIntent(), SkipPreroll: true,
			Replacement: &PlaybackReplacementRequest{SourceSessionID: source.SessionID, RequestID: "receiver-handoff-living-room-1", PreviousTerminal: *playbackHandoffTerminalForTest(source, "stopped", 1), ExpectedQueueRevision: &source.QueueRevision, ExpectedPlaybackRevision: &source.PlaybackRevision},
		},
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/handoff", handoffRequest, &handedOff)
	if status != http.StatusOK || handedOff.SessionID == "" || handedOff.SessionID == source.SessionID {
		t.Fatalf("receiver handoff status=%d body=%s playback=%#v", status, body, handedOff)
	}
	if handedOff.CurrentQueueEntryID != source.CurrentQueueEntryID || len(handedOff.Queue) != 2 || handedOff.Queue[0].EntryID != "duplicate-forward-1" || handedOff.Queue[1].EntryID != "duplicate-forward-2" {
		t.Fatalf("receiver preparation did not preserve authoritative duplicate queue occurrences: %#v", handedOff.Queue)
	}
	var sourceEnded, sourceState, receiverState string
	_ = server.db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = ?`, source.SessionID).Scan(&sourceEnded, &sourceState)
	_ = server.db.QueryRow(`SELECT state FROM playback_sessions WHERE id = ?`, handedOff.SessionID).Scan(&receiverState)
	if sourceEnded != "" || sourceState == "stopped" || receiverState != "handoff_pending" {
		t.Fatalf("prepare committed authority early source=%q/%q receiver=%q", sourceEnded, sourceState, receiverState)
	}
	var preparedRetry PlaybackResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/handoff", handoffRequest, &preparedRetry)
	if status != http.StatusOK || preparedRetry.SessionID != handedOff.SessionID {
		t.Fatalf("receiver prepare exact retry status=%d body=%s playback=%#v", status, body, preparedRetry)
	}
	var handoffStatus PlaybackReceiverHandoffStatusResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/receivers/"+receiver.ID+"/handoffs/receiver-handoff-living-room-1?authorizationId="+grant.AuthorizationID+"&receiverPublicKeyFingerprint="+receiver.ReceiverPublicKeyFingerprint+"&sourceSessionId="+source.SessionID, nil, &handoffStatus)
	if status != http.StatusOK || handoffStatus.Outcome != "pending" || handoffStatus.ReceiverSessionID != "" {
		t.Fatalf("receiver handoff status reconciliation status=%d body=%s result=%#v", status, body, handoffStatus)
	}
	commit := PlaybackReceiverHandoffCommitRequest{AuthorizationID: grant.AuthorizationID, ReceiverPublicKeyFingerprint: receiver.ReceiverPublicKeyFingerprint,
		SourceSessionID: source.SessionID, ReceiverSessionID: handedOff.SessionID, Readiness: "playing"}
	premature := commit
	premature.Readiness = "ready"
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/handoffs/receiver-handoff-living-room-1/commit", premature, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("premature receiver readiness commit status=%d body=%s", status, body)
	}
	wrongSession := commit
	wrongSession.ReceiverSessionID = "play_wrong_receiver_session"
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/handoffs/receiver-handoff-living-room-1/commit", wrongSession, nil)
	if status != http.StatusConflict {
		t.Fatalf("wrong receiver session commit status=%d body=%s", status, body)
	}
	var wrongReceiver PlaybackReceiver
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers", PlaybackReceiverRequest{
		ReceiverID: "receiver-bedroom", Name: "Bedroom", Platform: "Android TV", ReceiverPublicKey: receiverPublicKeyForTest(t),
		ClientInstanceID: "receiver-client-bedroom", SupportedCommands: []string{"load", "play"},
	}, &wrongReceiver)
	if status != http.StatusCreated {
		t.Fatalf("register wrong target status=%d body=%s", status, body)
	}
	var wrongGrant ReceiverControllerGrant
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+wrongReceiver.ID+"/authorizations", ReceiverAuthorizationRequest{
		RequestID: "authorize-bedroom-1", ControllerID: "controller-phone-1", ControllerPublicKey: controllerKey,
		AllowedCommands: []string{"load", "play"}, ClientInstanceID: authorize.ClientInstanceID,
	}, &wrongGrant)
	if status != http.StatusCreated {
		t.Fatalf("authorize wrong target status=%d body=%s", status, body)
	}
	wrongTarget := commit
	wrongTarget.AuthorizationID = wrongGrant.AuthorizationID
	wrongTarget.ReceiverPublicKeyFingerprint = wrongReceiver.ReceiverPublicKeyFingerprint
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+wrongReceiver.ID+"/handoffs/receiver-handoff-living-room-1/commit", wrongTarget, nil)
	if status != http.StatusConflict {
		t.Fatalf("wrong receiver target commit status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/handoffs/receiver-handoff-living-room-1/commit", commit, &handedOff)
	if status != http.StatusOK {
		t.Fatalf("receiver handoff commit status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/handoffs/receiver-handoff-living-room-1/commit", commit, &handedOff)
	if status != http.StatusOK {
		t.Fatalf("receiver handoff exact commit retry status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/receivers/"+receiver.ID+"/handoffs/receiver-handoff-living-room-1?authorizationId="+grant.AuthorizationID+"&receiverPublicKeyFingerprint="+receiver.ReceiverPublicKeyFingerprint+"&sourceSessionId="+source.SessionID, nil, &handoffStatus)
	if status != http.StatusOK || handoffStatus.Outcome != "accepted" || handoffStatus.ReceiverSessionID != handedOff.SessionID {
		t.Fatalf("receiver committed handoff status=%d body=%s result=%#v", status, body, handoffStatus)
	}
	var preservedHistory int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM playback_session_history WHERE session_id = ? AND media_id = ?`, handedOff.SessionID, source.Media.ID).Scan(&preservedHistory)
	if preservedHistory != 2 {
		t.Fatalf("receiver history occurrences collapsed: %d", preservedHistory)
	}
	var firstHistoryEntry, secondHistoryEntry string
	_ = server.db.QueryRow(`SELECT entry_id FROM playback_session_history WHERE session_id = ? AND history_id = ?`, handedOff.SessionID, "duplicate-history-1").Scan(&firstHistoryEntry)
	_ = server.db.QueryRow(`SELECT entry_id FROM playback_session_history WHERE session_id = ? AND history_id = ?`, handedOff.SessionID, "duplicate-history-2").Scan(&secondHistoryEntry)
	if firstHistoryEntry != "duplicate-past-1" || secondHistoryEntry != "duplicate-past-2" {
		t.Fatalf("receiver history identities changed: %q %q", firstHistoryEntry, secondHistoryEntry)
	}
	var restored PlaybackRestoreResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/active", PlaybackRestoreRequest{ClientInstanceID: "receiver-client-living-room", ClientProfile: authenticatedPlaybackRuntimeProfile()}, &restored)
	if status != http.StatusOK || !restored.Active || restored.Playback == nil || restored.Playback.SessionID != handedOff.SessionID {
		t.Fatalf("receiver restore status=%d body=%s restored=%#v", status, body, restored)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback/receivers/"+receiver.ID, PlaybackReceiverHeartbeatRequest{ReceiverPublicKeyFingerprint: "wrong"}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("mismatched receiver key heartbeat status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/playback/receivers/"+receiver.ID+"/authorizations/"+grant.AuthorizationID, nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("revoke receiver authorization status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback/receivers/"+receiver.ID, PlaybackReceiverHeartbeatRequest{ReceiverPublicKeyFingerprint: receiver.ReceiverPublicKeyFingerprint}, &heartbeat)
	if status != http.StatusOK || len(heartbeat.Authorizations) != 0 {
		t.Fatalf("revoked receiver heartbeat status=%d body=%s heartbeat=%#v", status, body, heartbeat)
	}

	var targets ListResponse[PlaybackTarget]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/targets", nil, &targets)
	if status != http.StatusOK || len(targets.Items) != 2 || targets.Items[0].Type != "receiver" || !strings.Contains(targets.Items[0].Detail, "Android TV") {
		t.Fatalf("receiver target status=%d body=%s targets=%#v", status, body, targets)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/command", PlaybackCommandRequest{Action: "load", MediaID: "movie_meridian"}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("legacy receiver command route status=%d body=%s", status, body)
	}
}

func TestPlaybackReceiverIsolationAndKeyRotationRevokeAuthorization(t *testing.T) {
	serverURL := newAuthTestServer(t)
	ownerJar, _ := cookiejar.New(nil)
	owner := &http.Client{Jar: ownerJar}
	loginUser(t, owner, serverURL)

	receiverKey := receiverPublicKeyForTest(t)
	var receiver PlaybackReceiver
	status, body := doJSON(t, owner, http.MethodPost, serverURL+"/api/playback/receivers", PlaybackReceiverRequest{
		ReceiverID: "receiver-key-rotation", Name: "Den", Platform: "Fire TV",
		ClientInstanceID:  "receiver-client-den",
		ReceiverPublicKey: receiverKey, SupportedCommands: []string{"load", "play", "pause", "seek", "stop"},
	}, &receiver)
	if status != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", status, body)
	}
	var grant ReceiverControllerGrant
	status, body = doJSON(t, owner, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/authorizations", ReceiverAuthorizationRequest{
		RequestID: "rotation-grant", ControllerID: "phone", ControllerPublicKey: receiverPublicKeyForTest(t), AllowedCommands: []string{"load", "play"},
		ClientInstanceID: "controller-client-phone",
	}, &grant)
	if status != http.StatusCreated {
		t.Fatalf("authorize status=%d body=%s", status, body)
	}

	rotatedKey := receiverPublicKeyForTest(t)
	var rotated PlaybackReceiver
	status, body = doJSON(t, owner, http.MethodPost, serverURL+"/api/playback/receivers", PlaybackReceiverRequest{
		ReceiverID: receiver.ID, Name: "Den", Platform: "Fire TV",
		ClientInstanceID:  "receiver-client-den",
		ReceiverPublicKey: rotatedKey, SupportedCommands: []string{"load", "play", "pause", "seek", "stop"},
	}, &rotated)
	if status != http.StatusCreated || rotated.ReceiverPublicKeyFingerprint == receiver.ReceiverPublicKeyFingerprint {
		t.Fatalf("rotate status=%d body=%s receiver=%#v", status, body, rotated)
	}
	var heartbeat PlaybackReceiverHeartbeatResponse
	status, body = doJSON(t, owner, http.MethodPatch, serverURL+"/api/playback/receivers/"+receiver.ID, PlaybackReceiverHeartbeatRequest{ReceiverPublicKeyFingerprint: rotated.ReceiverPublicKeyFingerprint}, &heartbeat)
	if status != http.StatusOK || len(heartbeat.Authorizations) != 0 {
		t.Fatalf("rotated receiver retained authorization status=%d body=%s heartbeat=%#v", status, body, heartbeat)
	}

	var viewer User
	status, body = doJSON(t, owner, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username: "receiver-viewer", Email: "receiver-viewer@example.test", DisplayName: "Receiver Viewer",
		Password: "Password1234", Role: "user", Permissions: permissionsForRole("user"), LibraryIDs: []string{"lib_movies"},
	}, &viewer)
	if status != http.StatusCreated {
		t.Fatalf("create viewer status=%d body=%s", status, body)
	}
	viewerJar, _ := cookiejar.New(nil)
	viewerClient := &http.Client{Jar: viewerJar}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": "receiver-viewer", "password": "Password1234"}, nil)
	if status != http.StatusOK {
		t.Fatalf("viewer login status=%d body=%s", status, body)
	}
	var receivers ListResponse[PlaybackReceiver]
	status, body = doJSON(t, viewerClient, http.MethodGet, serverURL+"/api/playback/receivers", nil, &receivers)
	if status != http.StatusOK || len(receivers.Items) != 0 {
		t.Fatalf("cross-profile receiver list status=%d body=%s receivers=%#v", status, body, receivers)
	}
}
