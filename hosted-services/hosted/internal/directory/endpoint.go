package directory

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"portico.local/hosted/internal/egress"
	"portico.local/hosted/internal/security"
	"strconv"
	"time"
)

type EndpointRequest struct {
	BaseURL        string `json:"baseUrl"`
	RegistrationID string `json:"registrationId"`
}
type EndpointChallenge struct {
	Kind                 string    `json:"kind"`
	ServerID             string    `json:"serverId"`
	AccountID            string    `json:"accountId"`
	OperationID          string    `json:"operationId"`
	ClaimGeneration      string    `json:"claimGeneration"`
	CredentialGeneration string    `json:"credentialGeneration"`
	BaseURL              string    `json:"baseUrl"`
	RegistrationID       string    `json:"registrationId"`
	Nonce                string    `json:"nonce"`
	ExpiresAt            time.Time `json:"expiresAt"`
}
type EndpointReceipt struct {
	BaseURL    string    `json:"baseUrl"`
	VerifiedAt time.Time `json:"verifiedAt"`
}
type EndpointEvidence struct {
	Generation int64
	BaseURL    string
	Digest     []byte
	ExpiresAt  time.Time
}
type EndpointRepository interface {
	RegisterEndpoint(context.Context, ServerAuthority, EndpointEvidence) (EndpointReceipt, error)
}

// ProofClient fetches a server's signed endpoint proof. It is injectable so a
// test can answer without a listening server; production uses the egress client
// that refuses private and loopback destinations.
type ProofClient interface {
	Do(*http.Request) (*http.Response, error)
}
type Options struct{ ProofClient ProofClient }

var ErrEndpointIdentity = errors.New("endpoint identity mismatch")
var ErrCapacity = errors.New("endpoint proof capacity reached")

func NewWithOptions(repo Repository, keys *security.Keys, options Options) *Service {
	client := options.ProofClient
	if client == nil {
		client = egress.NewServerClient(false)
	}
	return &Service{repo: repo, keys: keys, proof: client, proofSlots: make(chan struct{}, 8)}
}

func (s *Service) RegisterEndpoint(ctx context.Context, a ServerAuthority, q EndpointRequest) (EndpointReceipt, error) {
	return s.registerEndpoint(ctx, a, q, 0)
}
func (s *Service) RegisterRouteEndpoint(ctx context.Context, a ServerAuthority, generation int64, q EndpointRequest) (EndpointReceipt, error) {
	if generation < 1 {
		return EndpointReceipt{}, ErrInvalid
	}
	out, e := s.registerEndpoint(ctx, a, q, generation)
	if e != nil {
		if repo, ok := s.repo.(RouteRepository); ok {
			state := "failed"
			if errors.Is(e, ErrEndpointIdentity) {
				state = "identity_mismatch"
			}
			_ = repo.RecordRouteFailure(ctx, a, generation, q.BaseURL, state)
		}
	}
	return out, e
}
func (s *Service) registerEndpoint(ctx context.Context, a ServerAuthority, q EndpointRequest, generation int64) (EndpointReceipt, error) {
	repo, ok := s.repo.(EndpointRepository)
	if !ok || !a.Valid() {
		return EndpointReceipt{}, ErrDenied
	}
	origin, _, err := egress.ServerOrigin(q.BaseURL, false)
	if err != nil || origin != q.BaseURL {
		return EndpointReceipt{}, ErrInvalid
	}
	registration, err := base64.RawURLEncoding.DecodeString(q.RegistrationID)
	if err != nil || len(registration) != 32 || base64.RawURLEncoding.EncodeToString(registration) != q.RegistrationID {
		return EndpointReceipt{}, ErrInvalid
	}
	select {
	case s.proofSlots <- struct{}{}:
		defer func() { <-s.proofSlots }()
	default:
		return EndpointReceipt{}, ErrCapacity
	}
	ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	challenge := EndpointChallenge{"portico.server.endpoint.v1", a.ServerID, a.WireAccountID(), a.OperationID, strconv.FormatInt(a.ClaimGeneration, 10), strconv.FormatInt(a.CredentialGeneration, 10), origin, q.RegistrationID, security.RandomID(""), time.Now().UTC().Add(30 * time.Second)}
	raw, err := json.Marshal(challenge)
	if err != nil {
		return EndpointReceipt{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/v1/networking/endpoint-proof", bytes.NewReader(raw))
	if err != nil {
		return EndpointReceipt{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := s.proof.Do(req)
	if err != nil {
		return EndpointReceipt{}, ErrDenied
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return EndpointReceipt{}, ErrDenied
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<10)+1))
	if err != nil || len(body) > 8<<10 {
		return EndpointReceipt{}, ErrDenied
	}
	var proof struct {
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}
	if json.Unmarshal(body, &proof) != nil {
		return EndpointReceipt{}, ErrDenied
	}
	canonical, _ := json.Marshal(proof)
	if !bytes.Equal(body, canonical) || proof.Payload != base64.RawURLEncoding.EncodeToString(raw) {
		return EndpointReceipt{}, ErrDenied
	}
	signature, err := base64.RawURLEncoding.DecodeString(proof.Signature)
	if err != nil || base64.RawURLEncoding.EncodeToString(signature) != proof.Signature || !ed25519.Verify(a.PublicKey, raw, signature) || !challenge.ExpiresAt.After(time.Now()) {
		return EndpointReceipt{}, ErrEndpointIdentity
	}
	digest := sha256.Sum256(raw)
	return repo.RegisterEndpoint(ctx, a, EndpointEvidence{BaseURL: origin, Digest: digest[:], ExpiresAt: challenge.ExpiresAt, Generation: generation})
}
