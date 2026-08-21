VERSION ?= 0.1.0
COMMIT ?= local
BUILT_AT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILD_NUMBER ?= 1
CHANNEL ?= development
SAFETY_CLASS ?= development
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildNumber=$(BUILD_NUMBER) -X main.channel=$(CHANNEL) -X main.commit=$(COMMIT) -X main.builtAt=$(BUILT_AT) -X main.releaseSafetyClass=$(SAFETY_CLASS)
SERVER_OUTPUT ?= dist/porticod

.PHONY: dev-api dev-api-tray dev-web test build-client-core build-web build-server build-server-tray performance-check performance-deep-check load-harness api-generate api-server-check api-check contract-check

api-generate:
	go run ./cmd/genopenapi
	cd packages/portico-client-core && npm run api:types:server

api-server-check:
	go run ./cmd/genopenapi -check
	@test -s api/openapi/portico-server.openapi.json
	@test -s internal/app/apiroute/contract.json
	@test -s packages/portico-client-core/src/operationContract.generated.ts

api-check:
	$(MAKE) api-server-check
	@tmp="$$(mktemp)"; trap 'rm -f "$$tmp"' EXIT; \
		packages/portico-client-core/node_modules/.bin/openapi-typescript api/openapi/portico-server.openapi.json -o "$$tmp" >/dev/null; \
		cmp -s "$$tmp" packages/portico-client-core/src/openapi-types.ts || { echo "Generated Server client types are stale; run make api-generate" >&2; exit 1; }

contract-check: api-server-check
	go test ./internal/app -run '^TestPublishedContract' -count=1

dev-api:
	go run ./cmd/porticod

dev-api-tray: # gitleaks:allow -- Make target name, not a credential
	CGO_ENABLED=1 go run -tags tray ./cmd/porticod --tray

dev-web: build-client-core
	cd web && npm run dev

test: api-check
	go test ./...
	$(MAKE) build-web

build-client-core:
	cd packages/portico-client-core && npm run build

build-web: build-client-core
	cd web && npm run build
	cd web && npm run verify:bundle

build-server:
	go build -trimpath -ldflags="$(LDFLAGS)" -o "$(SERVER_OUTPUT)" ./cmd/porticod

build-server-tray:
	CGO_ENABLED=1 go build -tags tray -trimpath -ldflags="$(LDFLAGS)" -o dist/porticod-tray ./cmd/porticod

performance-check:
	./scripts/performance-release-check.sh

performance-deep-check:
	./scripts/performance-deep-check.sh

load-harness:
	go run ./scripts/load-harness --base-url "$${PORTICO_LOAD_BASE_URL:-http://127.0.0.1:32500}" --login "$${PORTICO_LOAD_LOGIN:-admin}" --password "$${PORTICO_LOAD_PASSWORD:?Set PORTICO_LOAD_PASSWORD}" --users "$${PORTICO_LOAD_USERS:-24}" --duration "$${PORTICO_LOAD_DURATION:-30s}" --profile "$${PORTICO_LOAD_PROFILE:-mixed}" --diagnostics-interval="$${PORTICO_LOAD_DIAGNOSTICS_INTERVAL:-0s}" --scan-during-run="$${PORTICO_LOAD_SCAN_DURING_RUN:-false}" --metadata-refresh-during-run="$${PORTICO_LOAD_METADATA_REFRESH_DURING_RUN:-false}" --live-tv-guide="$${PORTICO_LOAD_LIVE_TV_GUIDE:-false}" --max-errors="$${PORTICO_LOAD_MAX_ERRORS:-0}" --max-p95-ms="$${PORTICO_LOAD_MAX_P95_MS:-0}" --max-sqlite-lock-retries="$${PORTICO_LOAD_MAX_SQLITE_LOCK_RETRIES:-0}" --max-workload-rejections="$${PORTICO_LOAD_MAX_WORKLOAD_REJECTIONS:-0}" --max-admission-rejections="$${PORTICO_LOAD_MAX_ADMISSION_REJECTIONS:-0}"
