# Makefile del repositorio de basekit (el CLI). `make help` lista los objetivos.
SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help
PKG ?= ./...
STACK ?=
DB ?=
# Passed through the environment and validated by the shell: never interpolated into a recipe by make,
# so a value like `x; rm -rf ~` cannot become a command (subagents may run these targets unattended).
export PKG STACK DB

.PHONY: help build verify test template-check templates-pack e2e zones agents agents-check release-dry

help: ## Lista los objetivos
	@grep -hE '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  \033[36m%-15s\033[0m %s\n",$$1,$$2}'

build: ## Compila bin/basekit
	go build -trimpath -o bin/basekit ./cmd/basekit

verify: ## gofmt + vet + staticcheck (si está) + tests, con salida mínima (PKG=./internal/x/...)
	@[[ "$$PKG" =~ ^\./[A-Za-z0-9_./-]*$$ ]] || { echo "PKG inválido: usa ./... o ./internal/<paquete>/..."; exit 2; }
	@bash .agents/skills/cli-verify/scripts/verify.sh

test: ## Todos los tests (con Docker incluye la aceptación 9)
	go test -count=1 ./...

template-check: ## Valida un stack de plantillas/ (STACK=<id>; los ids: go run ./cmd/basekit stacks)
	@[[ "$$STACK" =~ ^[a-z0-9][a-z0-9-]*$$ ]] || { echo "Uso: make template-check STACK=<id>"; exit 2; }
	@bash .agents/skills/template-check/scripts/check.sh "$$STACK"

templates-pack: ## Regenera internal/stack/modules.tar (plantillas con go.mod propio, que go:embed no incluye)
	go test -count=1 -run TestModulesArchiveIsCurrent ./internal/stack -args -update >/dev/null || true
	go test -count=1 -run TestModulesArchiveIsCurrent ./internal/stack

e2e: ## Pruebas con Docker real, dev y prod (STACK=<id> o vacío para todos; DB=postgres|mysql|sqlite|none)
	@[[ -z "$$STACK" || "$$STACK" =~ ^[a-z0-9][a-z0-9-]*$$ ]] || { echo "STACK inválido"; exit 2; }
	@[[ -z "$$DB" || "$$DB" =~ ^(postgres|mysql|sqlite|none)$$ ]] || { echo "DB inválido: postgres, mysql, sqlite o none"; exit 2; }
	BASEKIT_E2E=1 BASEKIT_E2E_STACK="$$STACK" BASEKIT_E2E_DB="$$DB" go test -count=1 -timeout 90m -v -run 'TestDevAnswersHealthcheck|TestProdAnswersHealthcheck' ./internal/cli

zones: ## Cada archivo cambiado tiene un agente dueño (zone-check --owners)
	@bash .agents/skills/zone-check/scripts/check_zones.sh --owners

agents: ## Regenera .claude/agents y .opencode/agents desde .agents/ y enlaza las skills en .claude/skills
	@bash scripts/gen-agents.sh

agents-check: ## Falla si .claude/agents o .opencode/agents no coinciden con los roles y zonas (CI)
	@bash scripts/gen-agents.sh >/dev/null
	@git diff --exit-code --stat -- .claude/agents .opencode/agents .claude/skills && \
	  [ -z "$$(git ls-files --others --exclude-standard -- .claude/agents .opencode/agents .claude/skills)" ] || \
	  { echo "Los adaptadores no están al día: ejecuta make agents y commitea el resultado"; exit 1; }

release-dry: ## Compila los binarios de release en dist/ sin publicar
	./scripts/build-release.sh 0.0.0-dev
