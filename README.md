# basekit (Go)

CLI que prepara la base de un proyecto para agentes de código (Claude Code, OpenCode, Codex, Cursor, Gemini CLI, Copilot, Hermes): `AGENTS.md`, roles y zonas, skills, Docker + `Makefile` y documentación. Es un binario único: las plantillas de `plantillas/` van embebidas con `go:embed`.

La especificación está en `docs/guias/02_especificacion_cli.md`. El seguimiento de la implementación está en `odd/tasks/basekit-go.md`.

## Instalación

El binario se instala **global** (queda en tu `PATH`) y el kit se instala **por proyecto** (por defecto) o **global**.

**Con un binario de release** (no necesita Go). Cada tag `v*` publica binarios para Windows, macOS y Linux (amd64 y arm64) con `checksums.txt`. Como el repo es privado, se descargan con `gh`:

```bash
# macOS / Linux (cambia el sistema y la arquitectura según tu equipo)
gh release download --repo F4R1DS4NCH3Z26/basekit-go --pattern '*darwin_arm64.tar.gz' --pattern checksums.txt
shasum -a 256 -c --ignore-missing checksums.txt
tar -xzf basekit_*_darwin_arm64.tar.gz && sudo mv basekit_*_darwin_arm64/basekit /usr/local/bin/
```

```powershell
# Windows (PowerShell)
gh release download --repo F4R1DS4NCH3Z26/basekit-go --pattern '*windows_amd64.zip'
Expand-Archive basekit_*_windows_amd64.zip -DestinationPath $env:LOCALAPPDATA\basekit
# agrega $env:LOCALAPPDATA\basekit\basekit_<versión>_windows_amd64 a tu PATH
```

**Con Go** (1.24 o superior):

```bash
# El repo es privado: Go necesita saberlo y git necesita tus credenciales de GitHub.
gh auth setup-git
GOPRIVATE=github.com/F4R1DS4NCH3Z26/* go install github.com/F4R1DS4NCH3Z26/basekit-go/cmd/basekit@latest
```

Queda en `$(go env GOPATH)/bin/basekit`. Para fijar una versión, cambia `@latest` por un tag o un commit.

### Windows

- **Recomendado: WSL2.** Instala basekit dentro de tu distribución (Ubuntu) y trabaja ahí. Los proyectos que genera usan `make`, `bash` y `openssl`, y Docker Desktop se integra con WSL2, así que todo funciona igual que en Linux.
- **Windows nativo:** `basekit.exe` funciona: `init`, `sync`, `doctor`, `--global` y las preguntas interactivas. Si Windows no permite crear symlinks (sin modo desarrollador), copia las skills en lugar de enlazarlas. El flujo de Docker y `Makefile` (`make dev`, `make env`) y `--db` necesitan `make` y `bash`, que Windows no trae: usa WSL2 o Git Bash con `make`.
- El CI corre los tests y una prueba del binario en Windows en cada cambio.

| Kit | Comando | Qué instala |
|---|---|---|
| Por proyecto (default) | `basekit init` dentro del proyecto | Todo: `AGENTS.md`, roles y zonas, skills del kit y del stack, adaptadores, Docker, `Makefile` y docs |
| Global | `basekit init --global` | Solo lo reutilizable: las skills del kit sin stack (`project-bootstrap`, `infra-setup`, `zone-check`) y las externas, en `~/.agents/skills`, enlazadas en la carpeta global de cada agente (`~/.claude/skills`, `~/.config/opencode/skills`, `~/.cursor/skills`, `~/.gemini/skills`, `~/.copilot/skills`, `~/.hermes/skills`; Codex lee `~/.agents/skills`) |

Lo específico de cada stack nunca es global: depende de las rutas y del framework de cada repo. Con la instalación global, en cada proyecto igual corres `basekit init`.

## Requisitos

- Go 1.24 o superior, solo para compilar o instalar con `go install`.
- En el proyecto destino: `git`, y también Docker, `make` y `openssl` si se usa Docker. Las skills externas necesitan Node ≥ 20 (`npx`).

## Compilar y probar

```bash
go build -o basekit ./cmd/basekit
go test ./...                          # con Docker incluye la aceptación 9 (compose config con BD)
BASEKIT_E2E=1 go test -run TestDevAnswersHealthcheck ./internal/cli   # aceptación 12: make new-app + make dev + healthcheck
```

La aceptación 12 construye las imágenes de cada stack y usa los puertos de `stack.json`. Esos puertos tienen que estar libres. `BASEKIT_E2E_STACK=<id>` la limita a un stack. El CI (`.github/workflows/ci.yml`) corre la suite y un job de aceptación 12 por stack.

## Uso

```bash
basekit init [dir]        # crea la base en un proyecto nuevo o existente
basekit init --global     # skills reutilizables para todos tus proyectos
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

Las preguntas usan [huh](https://github.com/charmbracelet/huh): flechas para moverte, espacio para marcar agentes, Enter para confirmar y Ctrl+C para cancelar sin escribir nada. Con `ACCESSIBLE=1` (lectores de pantalla), o si la entrada no es una terminal, pasan a ser preguntas de texto plano, una por línea.

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

## Estructura

```
cmd/basekit/main.go     # arranque: detecta la terminal y llama a cli.Main
internal/
├── cli/                # entrega: flags, comandos, ayuda y preguntas en la terminal (+ tests de caja negra)
├── scaffold/           # núcleo: init, sync, doctor y el orden de las preguntas
├── stack/              # stack.json, detección y plantillas embebidas
├── manifest/           # .agents/manifest.json y el pin de las skills externas
├── adapters/           # agentes soportados, adaptadores por herramienta y enlaces de skills
├── fsx/                # única puerta al disco: rutas confinadas, idempotencia, dry-run
└── usage/              # error de uso (exit 2)
plantillas/             # datos: todo lo específico de cada stack
```

Las dependencias apuntan hacia adentro: `cli` → `scaffold` → `stack`, `manifest` y `adapters` → `fsx`. `scaffold` declara la interfaz que consume (`Prompter`) y `cli` la implementa con la terminal.

## Agregar un stack

Crea una carpeta en `plantillas/stacks/<id>/` con su `stack.json` (incluido el campo `language`) y vuelve a compilar. El código del CLI no contiene nada específico de un stack.
