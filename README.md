# basekit (Go)

CLI que prepara la base de un proyecto para agentes de código (Claude Code, OpenCode, Codex, Cursor, Gemini CLI, Copilot, Hermes): `AGENTS.md`, roles y zonas, skills, Docker + `Makefile` y documentación. Es un binario único: las plantillas de `plantillas/` van embebidas con `go:embed`.

La especificación está en `docs/guias/02_especificacion_cli.md`. El seguimiento de la implementación está en `odd/tasks/basekit-go.md`.

## Requisitos

- Go 1.24 o superior para compilar.
- En el proyecto destino: `git`, y también Docker, `make` y `openssl` si se usa Docker. Las skills externas necesitan Node ≥ 20 (`npx`).

## Compilar y probar

```bash
go build -o basekit .
go test ./...                          # con Docker incluye la aceptación 9 (compose config con BD)
BASEKIT_E2E=1 go test -run TestDevAnswersHealthcheck ./internal/basekit   # aceptación 12: make new-app + make dev + healthcheck
```

La aceptación 12 construye las imágenes de cada stack y usa los puertos de `stack.json`. Esos puertos tienen que estar libres. `BASEKIT_E2E_STACK=<id>` la limita a un stack. El CI (`.github/workflows/ci.yml`) corre la suite y un job de aceptación 12 por stack.

## Uso

```bash
basekit init [dir]        # crea la base en un proyecto nuevo o existente
basekit sync [dir]        # regenera el bloque de AGENTS.md, los adaptadores y los enlaces
basekit doctor [dir]      # diagnóstico OK / FALTA
basekit stacks            # stacks, lenguajes y agentes soportados
basekit help
```

### Preguntas de `init`

En una terminal, `init` pregunta solo lo que falte, en el orden en que el proyecto lo necesita:

1. **Lenguaje**: filtra los templates.
2. **Template** (stack): se elige solo si el lenguaje tiene uno único.
3. **Skills**: muestra las incluidas (kit y stack) y pregunta si se instalan las externas (`grill-me`, `grill-with-docs`, `grilling`, `domain-modeling`).
4. **Agentes**: herramientas para las que se generan adaptadores.
5. **Docker + Makefile**.
6. **`git init`**: solo si no existe `.git`.
7. **Contexto**: de qué trata el proyecto.

Una pregunta se salta cuando la responde un flag, el manifest (`.agents/manifest.json`) o la detección: archivos del stack, carpetas de agentes o un `.git` existente. Con `--yes`, o sin terminal (CI), no pregunta nada: usa los flags y los valores por defecto.

### Flags de `init`

| Flag | Efecto |
|---|---|
| `--stack <id>` | Template: `nextjs`, `fastapi`, `laravel`, `spring` |
| `--agents <a,b>` | Agentes (por defecto `claude-code,opencode`) |
| `--context <texto>` | De qué trata el proyecto |
| `--db <motor>` | `auto`, `postgres`, `mysql`, `sqlite`, `none` |
| `--redis` | Agrega Redis |
| `--no-docker` | Sin Docker ni `Makefile` (no compatible con `--db` ni `--redis`) |
| `--no-git` | No ejecuta `git init` |
| `--no-external-skills` | No instala las skills externas |
| `--force` | Reemplaza las skills del kit modificadas localmente |
| `--dry-run` | Muestra lo que haría sin escribir ni ejecutar nada |
| `-y`, `--yes` | No interactivo |

## Agregar un stack

Crea una carpeta en `plantillas/stacks/<id>/` con su `stack.json` (incluido el campo `language`) y vuelve a compilar. El código del CLI no contiene nada específico de un stack.
