package networking

import "portico.local/server/internal/hostedtrust"

// PolicyEnvelope is a Hosted-signed document (claim cleanup results). Hosted
// no longer distributes a membership policy: membership is this server's own
// (Spec — Hosted at Scale), so there is nothing here to fetch.
type PolicyEnvelope = trust.Envelope
