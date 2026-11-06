# Makefile del repositorio de basekit (el CLI). `make help` lista los objetivos.
SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help
PKG ?= ./...
STACK ?=

.PHONY: help build verify test template-check e2e zones agents release-dry

help: ## Lista los objetivos
	@grep -hE '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  \033[36m%-15s\033[0m %s\n",$$1,$$2}'

build: ## Compila bin/basekit
	go build -trimpath -o bin/basekit ./cmd/basekit

verify: ## gofmt + vet + staticcheck (si está) + tests, con salida mínima (PKG=./internal/x/...)
	@PKG='$(PKG)' bash .agents/skills/cli-verify/scripts/verify.sh

test: ## Todos los tests (con Docker incluye la aceptación 9)
	go test -count=1 ./...

template-check: ## Valida un stack de plantillas/ (STACK=nextjs|fastapi|laravel|spring)
	@[ -n "$(STACK)" ] || { echo "Uso: make template-check STACK=<id>"; exit 2; }
	@bash .agents/skills/template-check/scripts/check.sh $(STACK)

e2e: ## Pruebas con Docker real, dev y prod (STACK=<id> o vacío para todos)
	BASEKIT_E2E=1 BASEKIT_E2E_STACK=$(STACK) go test -count=1 -timeout 90m -v -run 'TestDevAnswersHealthcheck|TestProdAnswersHealthcheck' ./internal/cli

zones: ## Cada archivo cambiado tiene un agente dueño (zone-check --owners)
	@bash .agents/skills/zone-check/scripts/check_zones.sh --owners

agents: ## Regenera .claude/agents y .opencode/agents desde .agents/roles y .agents/zones
	@bash scripts/gen-agents.sh

release-dry: ## Compila los binarios de release en dist/ sin publicar
	./scripts/build-release.sh 0.0.0-dev
