# AGENTS.md — basekit (CLI en Go)

Reglas para cualquier agente que trabaje en **este repositorio** (el CLI). Lo que el CLI genera en otros proyectos está en `plantillas/` y en `docs/guias/`.

Idea central: **los agentes se reparten territorio, no tareas. El contrato manda, los scripts verifican y todo corre con `make`.**

## Arquitectura
Dependencias en una sola dirección: `cli` → `scaffold` → `adapters` · `stack` · `manifest` → `fsx` · `usage`.

| Paquete | Qué es |
|---|---|
| `cmd/basekit` | Arranque: detecta la terminal y llama a `cli.Main` |
| `internal/cli` | Flags, comandos, ayuda y preguntas (huh o texto) |
| `internal/scaffold` | `init`, `init --global`, `sync`, `doctor`; procesos en `run.go` y `external.go` |
| `internal/adapters` | Agentes soportados y adaptadores por herramienta |
| `internal/stack` · `manifest` · `fsx` · `usage` | Plantillas embebidas, manifest, única puerta al disco, errores de uso |
| `plantillas/` | Lo que reciben los proyectos (embebido en el binario) |

## Agentes y zonas (`.agents/zones/<rol>.txt`)
| Agente | Zona | Rol |
|---|---|---|
| orquestador (sesión principal) | `AGENTS.md`, `docs/`, `odd/`, `.agents/`, `.claude/`, `.opencode/`, `.github/`, `scripts/`, `Makefile`, `go.mod`, `plantillas/embed.go` | Contratos, coordinación y cierre |
| `core-writer` | `internal/{fsx,stack,manifest,usage}` | `.agents/roles/core-writer.md` |
| `adapters-writer` | `internal/adapters`, `scaffold/{run,external}.go` | `.agents/roles/adapters-writer.md` |
| `commands-writer` | `cmd/`, `internal/cli/{cli,terminal,huh}.go`, `scaffold/{init,global,sync,doctor,agentsmd}.go` | `.agents/roles/commands-writer.md` |
| `templates-writer` | `plantillas/{core,skills,stacks}` | `.agents/roles/templates-writer.md` |
| `qa-writer` | `**/*_test.go`, `testdata/` | `.agents/roles/qa-writer.md` |

## Flujo de una feature
1. **Contrato** `docs/specs/<feature>_contract.md` (copia `docs/specs/_template_contract.md`): lo redacta el orquestador y lo cierra con el usuario (`Estado: cerrado`).
2. `make zones` limpio → `bash .agents/skills/zone-check/scripts/check_zones.sh --snapshot`.
3. **qa-writer** deja los tests en RED → **core-writer** → **adapters-writer** y **templates-writer** en paralelo si aplican → **commands-writer**. Tras cada uno: `check_zones.sh <rol>`.
4. **Cierre**: `make verify` + `make template-check STACK=<id>` si cambió `plantillas/` (+ `make e2e STACK=<id>` si cambió Docker) + `make zones`.

## Reglas globales
1. Edita solo tu zona. Una dependencia nueva (`go get`), un cambio de contrato o un archivo de otra zona → `ESTADO: BLOQUEADO`.
2. Test primero: cada regla con un test que falle antes del cambio. Los escritores no tocan `*_test.go`.
3. Nada específico de un stack en el código Go; todo sale de `plantillas/stacks/<id>/stack.json`.
4. Sin `sh -c` con datos del usuario; nunca leer `.env`; `--dry-run` no escribe ni lanza procesos.
5. Versiones fijadas con al menos 14 días de antigüedad. README y ayuda en español; código y comentarios en inglés.
6. Cambiaste `.agents/roles` o `.agents/zones` → `make agents` (regenera `.claude/agents` y `.opencode/agents`).

## Comandos
`make help` · `make verify [PKG=…]` · `make test` · `make template-check STACK=<id>` · `make e2e STACK=<id>` · `make zones` · `make agents` · `make build`

## Reporte de subagente (único formato)
`ESTADO: OK|FALLO|BLOQUEADO · ARCHIVOS: … · VERIFY: … · CONTRATO: sin cambios|requiere cambio: … · NOTAS: ≤2 líneas`
