# basekit CLI in Go

Objective: `basekit` in Go as a single binary with embedded templates and an interactive wizard ordered by project needs. Independent product: it does not track the Node/TS version in `~/Documents/guiade-project` (L7).
Branch: none (`project-guiade` is untracked inside the `~/Documents` repo; user memory: no commits without explicit approval). Delivery: ask-on-risk. Test runner: `go test ./...` (Go 1.27, zero third-party dependencies).

## Specs

- S1. Request (L1): "crea un cli en GO, la cual pregunte cosas como el lenguaje, skills, template, si va ser por docker y makefile, si se inicializa con git (init), (estas pregintas ordenalas segun la que necsite el proyecto)". Scope decision (L2): "Portar basekit a Go (Recommended)": started from the Node implementation, the same `plantillas/` and spec `docs/guias/02_especificacion_cli.md`. L7: "eat version es completamnete aparte a la del node" — from now on the Go version evolves on its own; Node changes are not ported and Node is no longer a parity baseline.
- S2. Templates are data (02 §6): "El CLI no debe tener nada específico de un stack en su código." Templates are embedded with `go:embed`; the language of each stack comes from a new `stack.json → language` field.
- S3. Commands and flags (02 §2): `init [dir]`, `sync [dir] [--agents a,b]`, `doctor [dir]`, `stacks`; `init` flags `--stack`, `--agents`, `--context`, `--db`, `--redis`, `--no-external-skills`, `--force`, `--dry-run`, `--yes`/`-y`.
- S4. `init` algorithm, mapping, placeholders, managed blocks and manifest as 02 §3–§8, with the marks "`+` (creado), `~` (actualizado), `=` (existía, se conserva) o `!` (aviso)".
- S5. Rules (02 §1): deterministic, idempotent ("`manifest.updatedAt` solo cambia si cambió algo más"), never overwrites user files, operates only inside the project dir.
- S6. Security (02 §12): whitelists, normalized paths, no shell with user data (exec with argument arrays), never read `.env`/`.env.prod`, relative symlinks with copy fallback, telemetry off, "`--dry-run` real: cero escrituras y cero procesos externos."
- S7. Adapters (03 §3) and skill links (02 §3 step 11), same output as the Node version.
- S8. External skills (02 §9), pinned `skills@1.7.4` + commit `5beb35b…`; a failure never aborts `init`.
- S9. `sync` (02 §10) and `doctor` (02 §11).
- S10. Acceptance tests 02 §13; 9 and 12 need Docker (CI).
- S11. Interactive wizard, only on a TTY without `--yes`, each question skipped when a flag, the manifest or detection already answers it. Order by dependency: 1 language → 2 template (filtered by language; auto when only one) → 3 skills (lists the kit and stack skills included; asks whether to install the external pack) → 4 agents → 5 Docker + Makefile → 6 `git init` (only when `.git` is missing) → 7 context. New flags: `--no-docker`, `--no-git`. Assumption: kit/stack skills are not deselectable because roles (`usa la skill nextjs-patterns`), `make verify` (`*-verify`) and the AGENTS.md flow depend on them.
- S12. `--no-docker`: skips `Makefile`, the stack `files/` and infra; is stored in `manifest.options.docker`; `--db`/`--redis` require Docker; warns that the AGENTS.md rules assume Docker. `--no-git` / answering no: no `git init`, warns that zone-check needs git.

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 | S2, S3 (`stacks`) | inline | done (uncommitted) | `go test` GREEN (L4) |
| T2 | S3 (`init`), S4, S5, S6 — acceptance 1–8 | inline | done (uncommitted) | `go test` GREEN + differential (L4) |
| T3 | S7, S8 — acceptance 10–11 | inline | done (uncommitted) | `go test` GREEN + differential (L4) |
| T4 | S9 | inline | done (uncommitted) | `go test` GREEN (L4) |
| T5 | S11, S12 | inline | done (uncommitted) | `go test` GREEN + pty smoke (L5) |
| T6 | S10 (9, 12) — CI with Docker | — | pending (needs Docker) | |

Route evidence: no Writer trigger (units depend on the shared core; no parallel independent units). Commits: none without user approval (overrides ODD work-unit commits).

## Log

- L1 (2026-11-05, user): "crea un cli en GO, la cual pregunte cosas como el lenguaje, skills, template, si va ser por docker y makefile, si se inicializa con git (init), (estas pregintas ordenalas segun la que necsite el proyecto)"
- L2 (2026-11-05, user): "Portar basekit a Go (Recommended)" — reuse `plantillas/` and spec 02, add the interactive flow.
- L3: Templates copied from `files/guias-basekit/plantillas/` to `plantillas/`. Template edits: `stack.json → language`; `npx basekit` → `basekit` (Go binary).
- L4 (T1–T4): RED observed (`go vet`: no non-test Go files), then GREEN: `go test ./...` → 44 top-level tests/subtests pass (ported Node suite: acceptance 1–8, 10, 11, sync, doctor, external skills with fake npx). Differential check vs Node `dist/src/cli.js` on the 4 stacks with all adapters: generated trees, symlink targets and file modes identical after normalizing `npx basekit` → `basekit`; manifests equal except the new `options.docker`. Engram mirror pending: auto mode blocked `mem_save` (a hook modifies the call).
- L5 (T5): RED observed (8 new wizard/`--no-docker`/`--no-git` tests failing), then GREEN: `go test ./...` → 53 top-level tests pass. Differential vs Node re-run after T5: non-interactive trees still identical. Pseudo-TTY smoke (`script`) of the real binary: questions in order Lenguaje → Template (auto) → Skills → Agentes → Docker + Makefile → git init → contexto. Decisions: the external-skills question is asked whenever they are missing (not persisted); `options.docker` is persisted and reruns keep it; `--db`/`--redis` without Docker is a usage error before any write. README.md (Spanish) added. Open decision: with no Docker, `AGENTS.md` rule 1–2 and roles still assume Docker/make (templates have no Docker-less variant); basekit only warns.
- L6 (verification, item 2 security guards): independent verifier → items 2–6 PASS; 2 blockers, both regressions vs the current Node baseline (`guiade-project/src/project.ts` gained `escapes`/`refuse` and the missing-end-marker guard at 16:41, after the snapshot this port read): (a) pre-existing symlinks inside the project (`docs`, `.agents`, `AGENTS.md`) let writes escape; (b) `block()` with a start marker and no end marker duplicated the block and later deleted user text. One correction batch: RED tests `TestSymlinkedDirsCannotLeadWritesOutside`, `TestBlockWithoutEndMarkerIsLeftAlone`, then GREEN (55 top-level tests). Recheck limited to those blockers with the real binary: 34 `enlace simbólico` warnings and 0 files outside; broken marker left intact on two runs. Verifier and parent scratch removed. Note: the Node baseline is still being edited (cli, external, init, prompt.ts, templates at 16:39–16:41); later Node changes are not ported.
- L7 (2026-11-05, user): "eat version es completamnete aparte a la del node" — S1 updated: the Go CLI is independent; the open item "later Node changes are not ported" is closed as intended. The L4–L6 differential checks remain as historical evidence only.
- L8 (2026-11-05, user): "elimina los archivos que no uses en este proyecto" — deleted `files/` after checking it held only duplicates: `files/0*.md` == `files/guias-basekit/0*.md`, the zip == the extracted folder, `files/guias-basekit/plantillas/` == `plantillas/` minus our edits, plus `.DS_Store`. Kept the only unique content, the guides, in `docs/guias/` (spec 00–05 + `stacks/*.md`); references updated.
