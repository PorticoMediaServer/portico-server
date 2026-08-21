package app

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPorticoAttachmentHandshakeProtectsBootstrapAndResponseOnHTTP(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	upsertJSONSetting(t, db, remoteAccessSettingsKey, map[string]any{
		"enabled": true, "claimStatus": "claimed", "serverId": "srv_attachment_test", "preferredRemoteAuthMode": "portico",
	})
	clientKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientNonce := make([]byte, 32)
	if _, err := rand.Read(clientNonce); err != nil {
		t.Fatal(err)
	}
	request := PorticoAttachmentHandshakeRequest{
		Version:         1,
		ClientPublicKey: base64.RawURLEncoding.EncodeToString(clientKey.PublicKey().Bytes()),
		ClientNonce:     base64.RawURLEncoding.EncodeToString(clientNonce),
	}
	response := performPorticoAttachmentHandshake(t, server, request)
	identity, err := server.loadOrCreateServerIdentity()
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(response.Signature)
	if err != nil || !ed25519.Verify(identity.PublicKey, porticoAttachmentTranscript(response), signature) {
		t.Fatal("attachment transcript did not verify with the durable server identity")
	}
	mutated := response
	mutated.Audience = "http://attacker.invalid"
	if ed25519.Verify(identity.PublicKey, porticoAttachmentTranscript(mutated), signature) {
		t.Fatal("an echoed fingerprint authenticated a substituted route transcript")
	}
	serverPublicBytes, err := base64.RawURLEncoding.DecodeString(response.ServerEphemeralPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	serverPublic, err := ecdh.P256().NewPublicKey(serverPublicBytes)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := clientKey.ECDH(serverPublic)
	if err != nil {
		t.Fatal(err)
	}
	transcript := porticoAttachmentTranscript(response)
	key := porticoAttachmentKey(shared, transcript)
	requestNonce := porticoAttachmentDerivedBytes("request-nonce", response.HandshakeID, response.ServerID, response.ServerPublicKeyFingerprint, 12)
	requestAAD := porticoAttachmentAAD("request", response.HandshakeID, response.ServerID, response.ServerPublicKeyFingerprint)
	bootstrap := "not-a-hosted-bootstrap-secret"
	plain, _ := json.Marshal(PorticoSessionAttachRequest{
		AccessToken:       bootstrap,
		SelectionEnvelope: HostedProfileSelectionEnvelope{AssertionID: "assertion", AccountID: "account", ProfileID: "profile", ServerID: response.ServerID},
		InstallationID:    "installation-attachment-test", DeviceName: "Test", App: "Portico Test", Platform: "Test",
	})
	ciphertext, err := sealPorticoAttachment(key, requestNonce, requestAAD, plain)
	if err != nil {
		t.Fatal(err)
	}
	encryptedRequest := PorticoAttachmentEncryptedRequest{Version: 1, HandshakeID: response.HandshakeID, Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext)}
	crossIPStatus, _, _ := performEncryptedPorticoAttachmentFromAddress(t, server, encryptedRequest, "192.168.1.99:43100")
	if crossIPStatus != http.StatusUnauthorized {
		t.Fatalf("cross-address attachment status=%d", crossIPStatus)
	}
	status, outerBody, encryptedResponse := performEncryptedPorticoAttachment(t, server, encryptedRequest)
	if status != http.StatusOK {
		t.Fatalf("encrypted invalid bootstrap status=%d body=%s", status, outerBody)
	}
	if strings.Contains(outerBody, bootstrap) || strings.Contains(outerBody, "server_session_revoked") {
		t.Fatalf("attachment transport exposed plaintext request or response: %s", outerBody)
	}
	responseCiphertext, err := base64.RawURLEncoding.DecodeString(encryptedResponse.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	responseNonce := porticoAttachmentDerivedBytes("response-nonce", response.HandshakeID, response.ServerID, response.ServerPublicKeyFingerprint, 12)
	responseAAD := porticoAttachmentAAD("response", response.HandshakeID, response.ServerID, response.ServerPublicKeyFingerprint)
	decrypted, err := openPorticoAttachment(key, responseNonce, responseAAD, responseCiphertext)
	var protected porticoAttachmentProtectedResponse
	if err != nil || json.Unmarshal(decrypted, &protected) != nil || protected.Status != http.StatusUnauthorized || !bytes.Contains(protected.Body, []byte(`"code":"server_session_revoked"`)) {
		t.Fatalf("decrypt attachment problem err=%v body=%s", err, decrypted)
	}
	replayStatus, replayBody, _ := performEncryptedPorticoAttachment(t, server, encryptedRequest)
	if replayStatus != status || replayBody != outerBody {
		t.Fatalf("exact lost-response retry minted a different result: status=%d body=%s", replayStatus, replayBody)
	}
}

func TestPorticoAttachmentHandshakeRejectsCiphertextTamperAndConsumesAttempt(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	upsertJSONSetting(t, db, remoteAccessSettingsKey, map[string]any{
		"enabled": true, "claimStatus": "claimed", "serverId": "srv_attachment_tamper", "preferredRemoteAuthMode": "portico",
	})
	clientKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	handshake := performPorticoAttachmentHandshake(t, server, PorticoAttachmentHandshakeRequest{
		Version: 1, ClientPublicKey: base64.RawURLEncoding.EncodeToString(clientKey.PublicKey().Bytes()), ClientNonce: base64.RawURLEncoding.EncodeToString(nonce),
	})
	tampered := PorticoAttachmentEncryptedRequest{Version: 1, HandshakeID: handshake.HandshakeID, Ciphertext: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}
	status, _, _ := performEncryptedPorticoAttachment(t, server, tampered)
	if status != http.StatusUnauthorized || server.porticoAttachmentHandshakeCount() != 0 {
		t.Fatalf("tampered attachment status=%d remaining=%d", status, server.porticoAttachmentHandshakeCount())
	}
}

func performPorticoAttachmentHandshake(t *testing.T, server *Server, request PorticoAttachmentHandshakeRequest) PorticoAttachmentHandshakeResponse {
	t.Helper()
	body, _ := json.Marshal(request)
	httpRequest := httptest.NewRequest(http.MethodPost, "http://192.168.1.20/api/auth/portico/handshakes", bytes.NewReader(body))
	httpRequest.RemoteAddr = "192.168.1.21:43100"
	recorder := httptest.NewRecorder()
	server.handlePorticoAttachmentHandshake(recorder, httpRequest)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create attachment handshake status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response PorticoAttachmentHandshakeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if expires, err := time.Parse(time.RFC3339Nano, response.ExpiresAt); err != nil || time.Until(expires) <= 0 {
		t.Fatalf("invalid attachment expiry %q: %v", response.ExpiresAt, err)
	}
	return response
}

func performProtectedPorticoAttachmentRequest(t *testing.T, server *Server, request PorticoSessionAttachRequest, response any) (int, string) {
	t.Helper()
	clientKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientNonce := make([]byte, 32)
	if _, err := rand.Read(clientNonce); err != nil {
		t.Fatal(err)
	}
	handshake := performPorticoAttachmentHandshake(t, server, PorticoAttachmentHandshakeRequest{
		Version: 1, ClientPublicKey: base64.RawURLEncoding.EncodeToString(clientKey.PublicKey().Bytes()), ClientNonce: base64.RawURLEncoding.EncodeToString(clientNonce),
	})
	serverPublicBytes, err := base64.RawURLEncoding.DecodeString(handshake.ServerEphemeralPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	serverPublic, err := ecdh.P256().NewPublicKey(serverPublicBytes)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := clientKey.ECDH(serverPublic)
	if err != nil {
		t.Fatal(err)
	}
	transcript := porticoAttachmentTranscript(handshake)
	key := porticoAttachmentKey(shared, transcript)
	plaintext, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := sealPorticoAttachment(
		key,
		porticoAttachmentDerivedBytes("request-nonce", handshake.HandshakeID, handshake.ServerID, handshake.ServerPublicKeyFingerprint, 12),
		porticoAttachmentAAD("request", handshake.HandshakeID, handshake.ServerID, handshake.ServerPublicKeyFingerprint),
		plaintext,
	)
	if err != nil {
		t.Fatal(err)
	}
	outerStatus, outerBody, encrypted := performEncryptedPorticoAttachment(t, server, PorticoAttachmentEncryptedRequest{
		Version: 1, HandshakeID: handshake.HandshakeID, Ciphertext: base64.RawURLEncoding.EncodeToString(sealed),
	})
	if outerStatus != http.StatusOK {
		t.Fatalf("protected attachment outer status=%d body=%s", outerStatus, outerBody)
	}
	responseCiphertext, err := base64.RawURLEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := openPorticoAttachment(
		key,
		porticoAttachmentDerivedBytes("response-nonce", handshake.HandshakeID, handshake.ServerID, handshake.ServerPublicKeyFingerprint, 12),
		porticoAttachmentAAD("response", handshake.HandshakeID, handshake.ServerID, handshake.ServerPublicKeyFingerprint),
		responseCiphertext,
	)
	if err != nil {
		t.Fatal(err)
	}
	var protected porticoAttachmentProtectedResponse
	if err := json.Unmarshal(decrypted, &protected); err != nil {
		t.Fatalf("decode protected attachment response: %v body=%s", err, decrypted)
	}
	if response != nil && len(protected.Body) > 0 {
		if err := json.Unmarshal(protected.Body, response); err != nil {
			t.Fatalf("decode attachment response body: %v body=%s", err, protected.Body)
		}
	}
	return protected.Status, string(protected.Body)
}

func performEncryptedPorticoAttachment(t *testing.T, server *Server, request PorticoAttachmentEncryptedRequest) (int, string, PorticoAttachmentEncryptedResponse) {
	return performEncryptedPorticoAttachmentFromAddress(t, server, request, "192.168.1.21:43100")
}

func performEncryptedPorticoAttachmentFromAddress(t *testing.T, server *Server, request PorticoAttachmentEncryptedRequest, remoteAddress string) (int, string, PorticoAttachmentEncryptedResponse) {
	t.Helper()
	body, _ := json.Marshal(request)
	httpRequest := httptest.NewRequest(http.MethodPost, "http://192.168.1.20/api/auth/portico/sessions", bytes.NewReader(body))
	httpRequest.RemoteAddr = remoteAddress
	recorder := httptest.NewRecorder()
	server.handlePorticoSessionAttach(recorder, httpRequest)
	var response PorticoAttachmentEncryptedResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	return recorder.Code, recorder.Body.String(), response
}
