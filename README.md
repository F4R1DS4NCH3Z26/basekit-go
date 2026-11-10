# basekit (Go)

CLI que prepara la base de un proyecto para agentes de código (Claude Code, OpenCode, Codex, Cursor, Gemini CLI, Copilot, Hermes): `AGENTS.md`, roles y zonas, skills, Docker + `Makefile` y documentación. Es un binario único: las plantillas de `plantillas/` van embebidas con `go:embed`.

La especificación está en `docs/guias/02_especificacion_cli.md`. El seguimiento de la implementación está en `odd/tasks/basekit-go.md`.

## Instalación

El binario se instala **global** (queda en tu `PATH`) y el kit se instala **por proyecto** (por defecto) o **global**.

**Con un binario de release** (no necesita Go). Cada tag `v*` publica binarios para Windows, macOS y Linux (amd64 y arm64) con `checksums.txt`. Como el repo es privado, se descargan con `gh`:

```bash
# macOS / Linux (cambia el sistema y la arquitectura según tu equipo)
gh release download --repo F4R1DS4NCH3Z26/basekit-go --pattern '*darwin_arm64.tar.gz' --pattern checksums.txt
shasum -a 256 -c --ignore-missing checksums.txt   # en Linux: sha256sum -c --ignore-missing checksums.txt
tar -xzf basekit_*_darwin_arm64.tar.gz && sudo mv basekit_*_darwin_arm64/basekit /usr/local/bin/
```

```powershell
# Windows (PowerShell)
gh release download --repo F4R1DS4NCH3Z26/basekit-go --pattern '*windows_amd64.zip' --pattern checksums.txt
$zip = Get-Item basekit_*_windows_amd64.zip
$want = (Select-String -Path checksums.txt -Pattern $zip.Name).Line.Split(' ')[0]
if ((Get-FileHash $zip -Algorithm SHA256).Hash -ne $want.ToUpper()) { throw "checksum distinto" }
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

La aceptación 12 construye las imágenes de cada stack y usa los puertos de `stack.json`. Esos puertos tienen que estar libres. `BASEKIT_E2E_STACK=<id>` la limita a un stack y `BASEKIT_E2E_DB=postgres|mysql|sqlite|none` elige la base de datos (también `make e2e STACK=<id> DB=<motor>`). La base de datos se decide en este orden:

1. Un stack frontend nunca tiene base de datos: un `BASEKIT_E2E_DB` explícito no se le aplica; corre sin ella y el test lo anota en el log.
2. `BASEKIT_E2E_DB=sqlite` omite Spring (guía 05, sección 3: Spring no ofrece SQLite), con un `SKIP` que lo explica.
3. Si no, un `BASEKIT_E2E_DB` explícito manda (`none`: sin base de datos).
4. Sin él: `postgres` para Spring, SQLite en prod y ninguna en dev. El CI (`.github/workflows/ci.yml`) corre la suite y un job de aceptación 12 por stack, en dev y en prod.

## Uso

```bash
basekit init [dir]        # crea la base en un proyecto nuevo o existente
basekit init --global     # skills reutilizables para todos tus proyectos
basekit sync [dir]        # regenera el bloque de AGENTS.md, los adaptadores y los enlaces
basekit doctor [dir]      # diagnóstico OK / FALTA
basekit stacks            # stacks (nombre, lenguaje y puerto) y agentes soportados
basekit help
```

### Preguntas de `init`

En una terminal, `init` pregunta solo lo que falte, en el orden en que el proyecto lo necesita:

1. **Stack**, en tres pasos, cada uno filtrado por el anterior: **tipo de app** (Backend, Frontend o Fullstack) → **lenguaje** → **framework** (la etiqueta es el `name` de `stack.json`). En Fullstack, el paso del lenguaje ofrece además **«Combinar backend + frontend»**: pregunta lenguaje y framework del backend y luego del frontend, y crea un proyecto combinado (abajo). Un paso con una sola opción se elige solo y se anuncia (`Lenguaje: TypeScript (única opción)`). En texto plano se acepta el número, el nombre o, en cualquier paso, el id de un stack (en los pasos de un lado, solo uno de ese lado).
2. **Skills**: muestra las incluidas (kit y stack) y pregunta `¿Qué skills externas instalo? (usan npx y red)`: una lista con `grill-me`, `grill-with-docs`, `grilling` y `domain-modeling`, todas marcadas. Se instalan solo las elegidas y la elección queda en el manifest (`options.externalSkills`): no se vuelve a preguntar y `sync` la respeta. Sin elegir ninguna, no se instala nada (`= skills externas  (ninguna elegida)`).
3. **Agentes**: herramientas para las que se generan adaptadores. Es opcional: sin ninguna se escriben `AGENTS.md`, `.agents/`, docs y Docker, pero no adaptadores ni enlaces de skills, y las skills externas esperan a que agregues un agente con `basekit sync --agents <a,b>` (necesitan `-a`).
4. **Docker + Makefile** (un proyecto combinado siempre los usa: no se pregunta).
5. **`git init`**: solo si no existe `.git`.
6. **Contexto**: de qué trata el proyecto.

Las preguntas usan [huh](https://github.com/charmbracelet/huh): flechas para moverte, espacio para marcar o desmarcar (skills externas y agentes; desmarcar todo vale como «ninguna»), Enter para confirmar y Ctrl+C para cancelar sin escribir nada. Con `ACCESSIBLE=1` (lectores de pantalla), o si la entrada no es una terminal, pasan a ser preguntas de texto plano, una por línea; en las listas se responde con números o nombres separados por coma, Enter deja los marcados por defecto y `ninguna` o `ninguno` no elige nada.

Una pregunta se salta cuando la responde un flag, el manifest (`.agents/manifest.json`) o la detección: archivos del stack, carpetas de agentes o un `.git` existente. Con `--yes`, o sin terminal (CI), no pregunta nada: usa los flags y los valores por defecto.

### Flags de `init`

| Flag | Efecto |
|---|---|
| `--stack <id>` | Stack (tabla de abajo; `basekit stacks` la imprime) |
| `--stack <back>+<front>` | Fullstack combinado: un stack backend en `backend/` y uno frontend en `frontend/` (por ejemplo `fastapi+react`) |
| `--agents <a,b>` | Agentes (por defecto `claude-code,opencode`); `--agents none`, ninguno (no se combina con ids) |
| `--context <texto>` | De qué trata el proyecto |
| `--db <motor>` | `auto`, `postgres`, `mysql`, `sqlite`, `none` (un stack frontend solo admite `auto` o `none`; en un combinado, va al backend) |
| `--redis` | Agrega Redis (no en un stack frontend; en un combinado, va al backend) |
| `--no-docker` | Sin Docker ni `Makefile` (no compatible con `--db`, `--redis` ni un combinado) |
| `--no-git` | No ejecuta `git init` |
| `--external-skills <a,b>` | Skills externas a instalar (`grill-me`, `grill-with-docs`, `grilling`, `domain-modeling`); `--external-skills none`, ninguna. Reemplaza la elección guardada sin preguntar e instala las elegidas que falten (las ya instaladas que quites se quedan). Vale igual en `sync`; con `--global`, solo para esa ejecución. No se combina con `--no-external-skills` |
| `--no-external-skills` | No instala las skills externas |
| `--force` | Reemplaza las skills del kit modificadas localmente |
| `--dry-run` | Muestra lo que haría sin escribir ni ejecutar nada |
| `-y`, `--yes` | No interactivo |

### Stacks

| Tipo | Lenguaje | `--stack` |
|---|---|---|
| Backend | Go | `gin`, `go-native` |
| Backend | Java | `spring` |
| Backend | PHP | `symfony`, `php-native` |
| Backend | Python | `fastapi`, `django`, `flask`, `python-native` |
| Backend | TypeScript | `nestjs`, `express`, `node-native` |
| Frontend | TypeScript | `react`, `vue`, `vanilla-ts` |
| Fullstack | PHP | `laravel` |
| Fullstack | Python | `django-htmx` |
| Fullstack | TypeScript | `nextjs` |

Los `*-native` (y `vanilla-ts`) no usan framework web. Cada stack se describe en `plantillas/stacks/<id>/stack.json` (`kind`, `language`, `name`…) y en su guía `docs/guias/stacks/<id>.md`. Un stack frontend no tiene base de datos: `init` rechaza `--db` (salvo `auto` o `none`) y `--redis`, y al cambiar un proyecto a un stack frontend se descartan la base de datos y Redis del manifest (los overlays `compose.db.yaml` y `compose.redis.yaml` quedan, porque son del usuario, e `init` avisa que ya no aplican). `express`, `django-htmx` y los nativos nunca se detectan solos: se eligen con `--stack` o en la pregunta. Si un proyecto tiene señales de dos stacks, gana el de menor `detect.priority` (guía 02, sección 6).

### Proyecto combinado: `backend/` + `frontend/`

`--stack <backend>+<frontend>` (solo un stack backend seguido de uno frontend) crea un monorepo. Cada carpeta es un proyecto completo de su stack (`Makefile`, `.make/stack.mk`, `Dockerfile`, `compose*.yaml`, `.env.example` y una copia de las skills de su stack) que funciona solo: `make -C backend dev`. La raíz tiene lo común una vez (`AGENTS.md` con las secciones de los dos stacks, `.agents/`, adaptadores, `docs/`) y un `Makefile` que delega en los dos: `make env && make dev` levanta ambos. Los roles y las zonas llevan el prefijo del lado (`backend-data-writer`), `--db` y `--redis` van al backend, el frontend recibe `VITE_API_URL` apuntando a la API y el backend permite su origen en `CORS_ORIGINS`. El manifest guarda `"stack": "fastapi+react"`; `sync` y `doctor` lo entienden. Detalle en la guía 02, sección 14, y la guía 04, sección 10.

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

## Desarrollo con agentes

El repositorio se trabaja con agentes que se reparten **territorio**: cada uno solo edita su zona (`.agents/zones/<rol>.txt`) y sigue su rol (`.agents/roles/<rol>.md`). Las reglas, la arquitectura y el flujo de una feature están en [`AGENTS.md`](AGENTS.md).

| Agente | Se encarga de |
|---|---|
| `core-writer` | `internal/fsx`, `stack`, `manifest`, `usage` |
| `adapters-writer` | `internal/adapters` y los procesos externos (`scaffold/run.go`, `external.go`) |
| `commands-writer` | `cmd/`, `internal/cli` y los comandos en `internal/scaffold` |
| `templates-writer` | `plantillas/` |
| `qa-writer` | Los tests |

En Claude Code se delegan por nombre (están en `.claude/agents/`); en OpenCode, en `.opencode/agents/` con la zona como permiso de edición. Cada feature empieza con un contrato (`docs/specs/_template_contract.md`) y se cierra con:

```bash
make verify                     # gofmt + vet + tests
make template-check STACK=<id>  # si cambió plantillas/
make zones                      # cada archivo cambiado tiene un agente dueño
```

Si cambias un rol o una zona, ejecuta `make agents` para regenerar `.claude/agents` y `.opencode/agents`.

## Agregar un stack

Crea una carpeta en `plantillas/stacks/<id>/` con su `stack.json` (con `kind`, `language`, `health` y `port`, que son obligatorios) y vuelve a compilar. El código del CLI no contiene nada específico de un stack. Las plantillas son las mismas que las de la versión en Node (ADR 0001): `go test ./internal/stack` lo comprueba contra `internal/stack/testdata/node-plantillas.tree`. Si una plantilla trae su propio `go.mod` (el starter de un stack de Go), `go:embed` no puede incluirla: ejecuta `make templates-pack` para regenerar `internal/stack/modules.tar`: escribe el archivo (`go test … -args -update`, que solo escribe y no compara contra la copia embebida, todavía vieja) y vuelve a correr el test sin `-update` para comprobar que quedó al día. Cualquier fallo se ve y corta el objetivo.
