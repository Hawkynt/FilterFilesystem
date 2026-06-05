# Agent guide — FilterFilesystem

Working agreement for **all** coding agents and human contributors working in
this repository. These rules are not optional. The full house spec lives in
the `Hawkynt/project-template` repo (`STANDARD.md`); this file is the
per-repo distillation.

## What this is

**FilterFS** — a Go filter filesystem (FUSE on Linux, WinFsp on Windows)
mounting directories with pattern-based visibility filtering. Layout:
`cmd/` (cobra CLI), `pkg/` (filesystem + filtering), `test/`, `Makefile`
(`make check` = fmt + lint via `.golangci.yml` + tests), `Dockerfile`.

## Commits

- **Group changes semantically/logically** — one backend/feature/concern per
  commit.
- **Every subject line starts with a prefix**: `+` added · `-` removed ·
  `*` changed · `#` bug fixed · `!` critical todo.
- Never start a subject with "fix"/"bugfix"/"changed"/"modified".
- **No AI traces anywhere**: no `Co-Authored-By` AI lines, no "Generated
  with" footers, no agent mentions in messages, comments, or authorship.

## The loop (always, in this order)

1. **Before committing**: `make check` until green (gofmt, golangci-lint and
   `go test ./...` — exactly what CI runs). Mount-behavior changes get
   exercised against a real mount where the platform allows. Update README
   pattern/CLI/config sections when flags change; `CHANGELOG.md` is
   generated — never edit it by hand.
2. **Commit** (rules above) and **push**.
3. **Wait for CI**; on `main` a green CI triggers the nightly (prerelease +
   GFS prune). Fix and loop until everything is green.

Stable releases are **manual** (`gh workflow run release.yml`) — never cut
one unless explicitly asked.

## Code conventions

- `gofmt` is law (formatter wins over house formatting rules); golangci-lint
  config in `.golangci.yml` is the lint contract.
- Platform parity matters: every Linux/FUSE feature states its WinFsp
  behavior (and vice versa) — errno mappings live in one place.
- Go is versioned by tags here (no version-bearing manifest) — never invent
  a VERSION file.

## README & repo conventions

- Standard frame: title → badges (incl. Go Report Card) → one-line `>`
  blockquote; fixed emoji mapping for the standard sections
  (`## ✨ Features`, `## 📦 Installation`, `## 🚀 Quick Start`,
  `## 🛠️ Development`, `## ❤️ Support`, `## 📜 License`).
- License is LGPL-3.0-or-later; the `## ❤️ Support` section and
  `.github/FUNDING.yml` stay intact.
