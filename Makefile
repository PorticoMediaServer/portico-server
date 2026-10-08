VERSION ?= 0.1.0
SERVER_OUTPUT ?= dist/portico-server

.PHONY: test test-go test-clients build-web build-server dev-api dev-web

# Go tests use a disposable PostgreSQL database for the packages that need one
# (PORTICO_TEST_DATABASE_URL_FILE); scripts/runner-test.sh sets that up on a runner.
test: test-go test-clients

test-go:
	cd apikit && go test ./... && go vet ./...
	cd server && go test ./... && go vet ./...

test-clients:
	npm run test:core
	npm run check:web

build-web:
	npm run build --workspace portico-web

build-server:
	cd server && CGO_ENABLED=0 go build -trimpath -o "$(abspath $(SERVER_OUTPUT))" ./cmd/server

dev-api:
	cd server && go run ./cmd/server

dev-web:
	npm run dev:web
