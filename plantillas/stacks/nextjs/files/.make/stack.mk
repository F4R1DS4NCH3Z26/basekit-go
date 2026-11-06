# Variables del stack Next.js para el Makefile común
APP         := app
PORT        := 3000
SHELL_CMD   := sh
INSTALL_CMD := npm ci
TEST_CMD    := npx --no-install vitest run
LINT_CMD    := npx --no-install eslint .
MAKE_MIGRATION_CMD = npx --no-install prisma migrate dev --name "$$NAME"
VERIFY_CMD  = bash .agents/skills/nextjs-verify/scripts/verify.sh "$${SCOPE:-src}"
CREATE_NEXT_VERSION ?= 16.4.2

# The guard and the work live in one shell decision: an `exit 0` on its own recipe line would only end that line.
new-app: ## Crea la app Next.js en Docker (solo si no existe package.json)
	@if [ -f package.json ]; then echo "Ya existe package.json; nada que hacer"; else $(MAKE) --no-print-directory _new-app; fi

_new-app:
	@# Runs as the image user (create-next-app needs a passwd entry) and hands the files back with chown.
	@# Copies each file that does not exist yet: BusyBox cp -n skips an existing destination dir and tar -k stops at the first existing file.
	docker run --rm -e UID_GID="$$(id -u):$$(id -g)" -e HOME=/tmp -e npm_config_cache=/tmp/.npm -e NEXT_TELEMETRY_DISABLED=1 \
	  -v "$$PWD":/work -w /tmp node:22-alpine sh -c '\
	  npx --yes create-next-app@$(CREATE_NEXT_VERSION) app --ts --eslint --app --src-dir --tailwind \
	    --import-alias "@/*" --use-npm --yes --skip-install && \
	  cd /tmp/app && find . ! -type d | while read -r f; do [ -e "/work/$$f" ] || { mkdir -p "/work/$$(dirname "$$f")" && cp -p "$$f" "/work/$$f"; }; done && \
	  test -f /work/package.json && \
	  cd /work && npm install --no-audit --no-fund && \
	  npm install --no-audit --no-fund zod server-only && \
	  npm install --no-audit --no-fund -D @types/node@^22 vitest @testing-library/react @testing-library/jest-dom jsdom && \
	  chown -R "$$UID_GID" /work'
	@f=$$(ls next.config.* 2>/dev/null | head -1); \
	if [ -n "$$f" ] && ! grep -q standalone "$$f"; then \
	  sed -i.bak 's|/\* config options here \*/|output: "standalone",|' "$$f" && rm -f "$$f.bak"; \
	  grep -q standalone "$$f" && echo "  ~ $$f: output standalone" || echo "AVISO: agrega output: \"standalone\" a $$f"; fi
	@mkdir -p src/app/api/health && [ -f src/app/api/health/route.ts ] || \
	  printf 'export function GET() {\n  return Response.json({ status: "ok" });\n}\n' > src/app/api/health/route.ts
	@echo "App creada. Siguiente: make env && make dev"
