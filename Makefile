APP_NAME := cex-router
GOENV := GOTOOLCHAIN=local GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org
GOVULNCHECK ?= $(shell go env GOPATH)/bin/govulncheck
SMOKE_EXCHANGES ?= bithumb,bitget,kucoin,gate,htx,coinex,whitebit,bitmart
NORMALIZATION_AUDIT_FLAGS ?=
DEMO_COIN ?= usdt
DEMO_FROM_CHAIN ?= ethereum
DEMO_TO_CHAIN ?= arbitrum
DEMO_OUTAGE_DURATION ?= 8s

.PHONY: build
build:
	$(GOENV) go build ./cmd/api ./cmd/ingester ./cmd/bot ./cmd/migrate ./cmd/smoke-adapters ./cmd/e2e-smoke ./cmd/normalization-audit ./cmd/alias-audit ./cmd/route-audit ./cmd/demo-outage

.PHONY: test
test:
	$(GOENV) go test ./...

.PHONY: fmt
fmt:
	gofmt -w cmd internal pkg

.PHONY: tidy
tidy:
	$(GOENV) go mod tidy

.PHONY: verify-deps
verify-deps:
	$(GOENV) go mod verify
	@if command -v govulncheck >/dev/null 2>&1; then \
		$(GOENV) govulncheck ./...; \
	elif [ -x "$(GOVULNCHECK)" ]; then \
		$(GOENV) "$(GOVULNCHECK)" ./...; \
	else \
		echo "govulncheck not installed; install with: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi

.PHONY: dev
dev:
	docker compose up --build db api ingester

.PHONY: dev-bot
dev-bot:
	docker compose --profile bot up --build bot

.PHONY: migrate
migrate:
	docker compose run --rm migrate up

.PHONY: smoke
smoke:
	$(GOENV) go test ./... -run Smoke

.PHONY: smoke-adapters
smoke-adapters:
	$(GOENV) go run ./cmd/smoke-adapters -env .env -exchanges $(SMOKE_EXCHANGES)

.PHONY: e2e-smoke
e2e-smoke:
	$(GOENV) go run ./cmd/e2e-smoke -env .env

.PHONY: normalization-audit
normalization-audit:
	$(GOENV) go run ./cmd/normalization-audit -env .env $(NORMALIZATION_AUDIT_FLAGS)

.PHONY: alias-audit
alias-audit:
	$(GOENV) go run ./cmd/alias-audit -env .env

.PHONY: route-audit
route-audit:
	$(GOENV) go run ./cmd/route-audit -env .env

.PHONY: demo
demo:
	./scripts/demo.sh

.PHONY: demo-outage
demo-outage:
	$(GOENV) go run ./cmd/demo-outage -env .env -coin $(DEMO_COIN) -from $(DEMO_FROM_CHAIN) -to $(DEMO_TO_CHAIN) -duration $(DEMO_OUTAGE_DURATION)

.PHONY: railway-demo-outage
railway-demo-outage:
	./scripts/railway-demo-outage.sh -coin $(DEMO_COIN) -from $(DEMO_FROM_CHAIN) -to $(DEMO_TO_CHAIN) -duration $(DEMO_OUTAGE_DURATION)

.PHONY: setup-hooks
setup-hooks:
	./scripts/install-hooks.sh
