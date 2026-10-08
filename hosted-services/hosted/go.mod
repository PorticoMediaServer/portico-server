module portico.local/hosted

go 1.26.2

require (
	github.com/coreos/go-oidc/v3 v3.21.0
	github.com/jackc/pgx/v5 v5.10.0
	golang.org/x/crypto v0.56.0
	golang.org/x/oauth2 v0.36.0
)

require (
	github.com/miekg/dns v1.1.72 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
)

require (
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	portico.local/apikit v0.0.0
	portico.local/dns v0.0.0
)

require portico.local/apikit v0.0.0

replace portico.local/dns => ../dns

replace portico.local/apikit => ../apikit
