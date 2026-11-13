# basekit (Go)

CLI que prepara la base de un proyecto para agentes de código (Claude Code, OpenCode, Codex, Cursor, Gemini CLI, Copilot, Hermes): `AGENTS.md`, roles y zonas, skills, Docker, `Makefile` y documentación. Es un binario único: las plantillas de `plantillas/` van embebidas con `go:embed`.

La especificación está en `docs/guias/02_especificacion_cli.md`. El seguimiento de la implementación está en `odd/tasks/basekit-go.md`.

## Instalación

El binario se instala **global** (queda en tu `PATH`) y el kit se instala **por proyecto** (por defecto) o **global**.

### Paso a paso

1. **Instala el binario** (no necesita Go):

   ```bash
   # macOS, Linux, WSL y Git Bash
   curl -fsSL https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.sh | sh
   ```

   ```powershell
   # Windows (PowerShell)
   irm https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.ps1 | iex
   ```

   Instala la última release (hoy `v0.3.0`) después de verificar su checksum, sin `sudo`: en `~/.local/bin`, o en `%LOCALAPPDATA%\Programs\basekit` en Windows.
2. **Deja `basekit` en tu `PATH`:** en macOS y Linux, si el instalador muestra una línea `export PATH=…`, agrégala a tu `~/.zshrc` o `~/.bashrc`. En Windows, el instalador ya agregó la carpeta a tu `PATH` de usuario.
3. **Abre una terminal nueva y verifica:** `basekit --version` imprime `0.3.0`.
4. **Prepara un proyecto:** `basekit init mi-proyecto` pregunta el tipo de app, el lenguaje y el framework; `basekit init mi-app --stack gin+react` crea un backend y un frontend juntos.

> **Usa una sola implementación.** Este CLI y [basekit (Node)](https://github.com/F4R1DS4NCH3Z26/basekit) instalan el mismo comando, `basekit`, y se ejecuta el que aparezca primero en tu `PATH`. Para quitar uno, ejecuta `basekit uninstall`: borra el `basekit` que corre (pregunta antes; `-y` no pregunta y `--dry-run` muestra qué borraría) y avisa si queda otra copia en el `PATH`. Con `--skills` también borra las skills globales de `basekit init --global`. Si no puedes ejecutarlo, a mano: borra `~/.local/bin/basekit` (en Windows, la carpeta `%LOCALAPPDATA%\Programs\basekit` y su entrada del `PATH` de usuario) o, para el de Node, `npm uninstall -g basekit`.

Si vas a usar `make dev` y Docker en Windows, instálalo dentro de WSL2 (ver [Windows](#windows)).

### Opciones y detalles

**Con el instalador** (no necesita Go). Cada tag `v*` publica binarios para Windows, macOS y Linux (amd64 y arm64) con `checksums.txt`. El instalador elige el de tu sistema, verifica su `sha256` contra `checksums.txt` antes de extraerlo y lo instala sin `sudo` ni permisos de administrador:

```bash
# macOS, Linux, WSL y Git Bash
curl -fsSL https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.sh | sh
# una versión y un directorio concretos
curl -fsSL https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.sh | sh -s -- --version v0.1.0 --dir ~/bin
```

```powershell
# Windows (PowerShell 5.1 o 7)
irm https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.ps1 | iex
# una versión y un directorio concretos
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.ps1))) -Version v0.1.0 -Dir C:\tools\basekit
```

| `install.sh` | `install.ps1` | Variable de entorno | Por defecto |
|---|---|---|---|
| `--version vX.Y.Z` | `-Version vX.Y.Z` | `BASEKIT_VERSION` | La última release |
| `--dir <ruta>` | `-Dir <ruta>` | `BASEKIT_INSTALL_DIR` | `~/.local/bin` · `%LOCALAPPDATA%\Programs\basekit` |
| — | — | `BASEKIT_DOWNLOAD_URL` | `https://github.com/F4R1DS4NCH3Z26/basekit-go/releases/download/<tag>` (para un espejo; acepta `file://`) |

- Si el checksum no coincide o falta en `checksums.txt`, no instala nada y termina con error.
- `install.sh` necesita `curl` o `wget` (también el de BusyBox, en Alpine), `tar` (o `unzip` en Git Bash) y `sha256sum` o `shasum`. Si el directorio no está en tu `PATH`, muestra la línea `export PATH=…` que debes agregar a tu shell.
- Si la descarga se corta, `curl … | sh` no ejecuta nada: todo el script está en funciones y solo la última línea, `{ main "$@"; }`, las llama (un corte dentro de ella deja las llaves sin cerrar y el shell no ejecuta nada).
- Cada descarga tiene límites de tiempo y se reintenta hasta 2 veces, pero no se empieza un reintento cuando ya pasaron 300 s desde el primer intento: con `curl`, 15 s para conectar y 300 s por intento (`--max-time 300 --retry-max-time 300`); con `wget`, 15 s sin respuesta y ningún límite por intento (el de BusyBox también respeta los 300 s; GNU `wget` solo cuenta los intentos); en PowerShell, `-TimeoutSec 300` por intento (según la versión, solo limita la espera de la respuesta, no una descarga que sigue recibiendo datos). Un intento que empezó antes de los 300 s sigue hasta su propio límite, así que con `curl` el peor caso es de unos 600 s (10 min) por descarga, más unos segundos de pausa entre intentos. Averiguar la última versión tiene sus propios límites: con `curl`, 60 s por intento y 60 s para reintentar (unos 120 s en el peor caso); en PowerShell, una sola petición de 60 s. Con `curl` 7.71 o más reciente se reintenta cualquier fallo (también una conexión rechazada, una descarga cortada o un 404); con un `curl` más antiguo, solo los que `curl` considera transitorios (tiempo agotado, HTTP 408, 429 o 5xx); con `wget` y en PowerShell, un 404 u otra respuesta 4xx no se reintenta. Si falla, el error muestra la URL y la causa (el mensaje de `curl`, `wget` o PowerShell); si con `curl` no se puede averiguar la última versión, también muestra la causa.
- `install.ps1` agrega el directorio a tu `PATH` de usuario: abre una terminal nueva para usar `basekit`.
- Si `irm … | iex` falla en una terminal abierta, muestra el error y deja `$LASTEXITCODE` en 1 sin cerrar la ventana. Si PowerShell ejecuta un solo comando y termina (`powershell -c "irm … | iex"`, `powershell "irm … | iex"`, `-EncodedCommand`, `-File` o `-NonInteractive`, sin `-NoExit`), lanza el error, así que el proceso sale con código 1 y un script o el CI ven el fallo.
- `BASEKIT_OS` y `BASEKIT_ARCH` existen solo para las pruebas (`make install-test` y `scripts/test-install.ps1`), que instalan sin red desde una release falsa.
- Estos instaladores son del CLI en Go. El basekit de Node se instala con npm (`basekit install`).

**A mano con `gh`.** Si prefieres no ejecutar un script descargado, baja y verifica la release tú mismo:

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
go install github.com/F4R1DS4NCH3Z26/basekit-go/cmd/basekit@latest
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

La aceptación 12 construye las imágenes de cada stack y usa los puertos de `stack.json`. Esos puertos tienen que estar libres; si uno está ocupado (por ejemplo, por otro servidor de desarrollo), `BASEKIT_E2E_PORT=<puerto>` publica la app en ese otro puerto del host. `BASEKIT_E2E_STACK=<id>` la limita a un stack y `BASEKIT_E2E_DB=postgres|mysql|sqlite|mongo|none` elige la base de datos (también `make e2e STACK=<id> DB=<motor>`). La base de datos se decide en este orden:

1. Un stack frontend nunca tiene base de datos: un `BASEKIT_E2E_DB` explícito no se le aplica; corre sin ella y el test lo anota en el log.
2. `BASEKIT_E2E_DB=sqlite` omite Spring (guía 05, sección 3: Spring no ofrece SQLite), con un `SKIP` que lo explica. Igual con cualquier otro motor que el stack no admite (`dbs` de su `stack.json`; `mongo` solo donde está en la lista): `SKIP` con los motores admitidos.
3. Si no, un `BASEKIT_E2E_DB` explícito manda (`none`: sin base de datos).
4. Sin él: `postgres` para Spring, SQLite en prod y ninguna en dev. El CI (`.github/workflows/ci.yml`) corre la suite en cada cambio y un job de aceptación 12 por stack, en dev y en prod, al pushear a `main` o a mano (`workflow_dispatch`), no en los PR.

## Uso

```bash
basekit init [dir]        # crea la base en un proyecto nuevo o existente
basekit init --global     # skills reutilizables para todos tus proyectos
basekit sync [dir]        # regenera el bloque de AGENTS.md, los adaptadores y los enlaces
basekit doctor [dir]      # diagnóstico OK / FALTA
basekit stacks            # stacks (nombre, lenguaje y puerto) y agentes soportados
basekit uninstall         # desinstala este basekit (--skills: también las skills globales)
basekit help
```

### Desinstalar

`basekit uninstall` muestra lo que va a borrar y pregunta `¿Desinstalar basekit?` (no, por defecto). Sin terminal necesita `-y` (o `--dry-run` para ver qué borraría). Borra solo el binario que corre; los proyectos que creaste no cambian. Si lo invocaste por un enlace simbólico, borra solo el enlace y conserva el binario. En Windows, si el binario está en la carpeta del instalador (`%LOCALAPPDATA%\Programs\basekit` o `BASEKIT_INSTALL_DIR`) y esa carpeta solo tiene basekit, también la quita del `PATH` de usuario y la carpeta se borra al terminar; si tiene otros archivos, solo se borra el `.exe`. Con `--skills` borra además lo que instaló `basekit init --global`: los archivos que lista `~/.agents/basekit.json`, los enlaces de basekit a esas skills en la carpeta global de cada agente y ese manifest; tus otras skills y los archivos que agregaste dentro de una skill no se tocan. Si algo no se puede borrar, lo avisa, conserva el manifest y el CLI y sale con 1 para que puedas reintentar. No funciona con `go run`: no hay nada instalado que borrar.

### Preguntas de `init`

En una terminal, `init` pregunta solo lo que falte, en el orden en que el proyecto lo necesita:

1. **Stack**, en tres pasos, cada uno filtrado por el anterior: **tipo de app** (Backend, Frontend o Fullstack) → **lenguaje** → **framework** (la etiqueta es el `name` de `stack.json`). En Fullstack, el paso del lenguaje ofrece además **«Combinar backend + frontend»**: pregunta lenguaje y framework del backend y luego del frontend, y crea un proyecto combinado (abajo). Un paso con una sola opción se elige solo y se anuncia (`Lenguaje: TypeScript (única opción)`). En texto plano se acepta el número, el nombre o, en cualquier paso, el id de un stack (en los pasos de un lado, solo uno de ese lado).
2. **Skills**: muestra las incluidas (kit y stack) y pregunta `¿Qué skills externas instalo? (usan npx y red)`: una lista con `grill-me`, `grill-with-docs`, `grilling` y `domain-modeling`, todas marcadas. Se instalan solo las elegidas y la elección queda en el manifest (`options.externalSkills`): no se vuelve a preguntar y `sync` la respeta. Sin elegir ninguna, no se instala nada (`= skills externas  (ninguna elegida)`).
3. **Agentes**: herramientas para las que se generan adaptadores. Es opcional: sin ninguna se escriben `AGENTS.md`, `.agents/`, docs y Docker, pero no adaptadores ni enlaces de skills, y las skills externas esperan a que agregues un agente con `basekit sync --agents <a,b>` (necesitan `-a`).
4. **Docker**: `¿Usar Docker? Todo corre en contenedores (Dockerfile y compose)` (por defecto, sí). Solo en la primera corrida, y no con `--db <motor>` ni `--redis`, que necesitan Docker. Un proyecto creado sin Docker lo recupera con `--docker`, que vuelve a montar la base de datos o Redis guardados; al apagarlo, `init` no borra nada, conserva la base de datos y Redis guardados y avisa con `!` los archivos de Docker que quedan.
5. **Makefile**, solo si respondiste que sí a Docker: `¿Usar Makefile? Es la interfaz para correr los comandos en los contenedores` (por defecto, sí). Las combinaciones posibles son Docker con `Makefile`, solo Docker o ninguno de los dos. Un proyecto combinado siempre usa los dos: no se pregunta ninguna de las dos cosas.
6. **Base de datos**: `¿Integrar base de datos?` (por defecto, sí) y, si respondes que sí, `¿Qué base de datos?`: MySQL, SQLite o PostgreSQL (por defecto), y MongoDB solo si el stack lo lista; si el stack declara `dbs` en su `stack.json`, solo esos (Spring: MySQL y PostgreSQL). Si respondes que no, la base de datos es `none`. La respuesta vale lo mismo que `--db <motor>`: infra-setup se ejecuta con ese motor y el manifest lo guarda en `options.db`. Solo se pregunta con Docker, con un stack que tiene base de datos (en un combinado, el backend), sin `--db` y solo en la primera corrida (sin manifest). En una nueva corrida se conserva el motor guardado; para cambiarlo, usa `--db`.
7. **`git init`**: solo si no existe `.git`.
8. **Contexto**: de qué trata el proyecto.

Las preguntas usan [huh](https://github.com/charmbracelet/huh): flechas para moverte, espacio para marcar o desmarcar (skills externas y agentes; desmarcar todo vale como «ninguna»), Enter para confirmar y Ctrl+C para cancelar sin escribir nada. Con `ACCESSIBLE=1` (lectores de pantalla), o si la entrada no es una terminal, pasan a ser preguntas de texto plano, una por línea; en las listas se responde con números o nombres separados por coma, Enter deja los marcados por defecto y `ninguna` o `ninguno` no elige nada.

Una pregunta se salta cuando la responde un flag, el manifest (`.agents/manifest.json`) o la detección: archivos del stack, carpetas de agentes o un `.git` existente. Con `--yes`, o sin terminal (CI), no pregunta nada: usa los flags y los valores por defecto.

### Flags de `init`

| Flag | Efecto |
|---|---|
| `--stack <id>` | Stack (tabla de abajo; `basekit stacks` la imprime) |
| `--stack <back>+<front>` | Fullstack combinado: un stack backend en `backend/` y uno frontend en `frontend/` (por ejemplo `fastapi+react`) |
| `--agents <a,b>` | Agentes (por defecto `claude-code,opencode`); `--agents none`, ninguno (no se combina con ids) |
| `--context <texto>` | De qué trata el proyecto |
| `--db <motor>` | `auto`, `postgres`, `mysql`, `sqlite`, `mongo`, `none` (un stack frontend solo admite `auto` o `none`; en un combinado, va al backend). Un stack sin `dbs` en su `stack.json` admite `postgres`, `mysql` y `sqlite`; `mongo` solo está disponible en los stacks que lo listan en `dbs`. Un stack con `dbs` solo admite esos motores: con Flask, `--db mongo` falla con `--db 'mongo' no aplica a django: motores admitidos: postgres, mysql, sqlite (o auto, none)`, y con Spring, `--db sqlite` falla con `--db 'sqlite' no aplica a spring: motores admitidos: postgres, mysql, mongo (o auto, none)` |
| `--redis` | Agrega Redis (no en un stack frontend; en un combinado, va al backend) |
| `--docker` | Vuelve a usar Docker en un proyecto creado sin Docker (no se combina con `--no-docker`) |
| `--no-docker` | Sin Docker ni `Makefile` (no compatible con `--redis`, un combinado ni `--db` distinto de `auto`/`none`) |
| `--no-makefile` | Docker sin `Makefile`: usa `docker compose` directo (no compatible con un combinado) |
| `--no-git` | No ejecuta `git init` |
| `--external-skills <a,b>` | Skills externas a instalar (`grill-me`, `grill-with-docs`, `grilling`, `domain-modeling`); `--external-skills none`, ninguna. Reemplaza la elección guardada sin preguntar e instala las elegidas que falten (las ya instaladas que quites se quedan). Vale igual en `sync`; con `--global`, solo para esa ejecución. No se combina con `--no-external-skills` |
| `--no-external-skills` | No instala las skills externas |
| `--force` | Reemplaza las skills del kit modificadas localmente |
| `--dry-run` | Muestra lo que haría sin escribir ni ejecutar nada |
| `-y`, `--yes` | No interactivo |

#### Docker sin `Makefile`

Con `--no-makefile`, o si respondes que no a la pregunta del `Makefile`, `init` escribe los archivos de Docker del stack (`Dockerfile`, `compose*.yaml`, `.dockerignore`, `.env.example`), pero no el `Makefile` ni nada en `.make/`, y avisa `! Makefile  (sin Makefile: usa docker compose directo, crea .env desde .env.example (reemplaza __GENERATE__) y adapta las reglas de AGENTS.md que usan make)`. La base de datos se configura igual (infra-setup no escribe `.make/db.mk` si no hay `Makefile`). El manifest guarda la elección junto a `docker`:

```json
"options": { "db": "postgres", "docker": true, "makefile": false, "redis": false }
```

Si ya está en el manifest, no se vuelve a preguntar; un manifest anterior a esta pregunta (sin `makefile`) cuenta como que sí, igual en `init`, `sync` y `doctor`. `sync` la respeta (`makefile` es `false` sin Docker o si el manifest dice `false`, y nunca escribe el `Makefile` ni `.make/`). `doctor` omite `make`, el `Makefile` y `.make/` (`·     sin Makefile (manifest): se omiten make y el Makefile`) y sigue revisando Docker.

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
