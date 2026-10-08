# Contributing to DrNetwork Panel

Thanks for helping. This page covers the development setup, the conventions the code follows, how to test, and how changes get merged.

## Contents

- [Setup](#setup)
- [Conventions](#conventions)
- [Testing](#testing)
- [Pull requests](#pull-requests)
- [Releases](#releases)
- [Reporting bugs and requesting features](#reporting-bugs-and-requesting-features)
- [Credits](#credits)

## Setup

### Prerequisites

- **Go** - the version in `go.mod` or newer.
- **Git**.
- **A C compiler** - CGO is required (`gcc`; `musl-dev` on Alpine).
- **Node.js** - only to work on or rebuild the frontend.

### Clone

Backend and frontend live in one repository; there is no submodule.

```bash
git clone https://github.com/Danialrostamani/drnetwork-panel
cd drnetwork-panel
```

The web panel is in [`frontend/`](frontend/) (Vue 3 + Vuetify + TypeScript). The compiled panel is embedded from `web/html`, which is not committed.

### Build and run

```bash
./runSUI.sh
```

This builds the frontend, copies it to `web/html`, builds the backend with the `dev` tags and starts it with `SUI_DB_FOLDER=db SUI_DEBUG=true`. The panel is at **http://localhost:2095/app/** (user `admin`, password `admin` - change it anywhere that matters).

Backend only, with the frontend already built:

```bash
rm -rf web/html/* && cp -R frontend/dist/* web/html/
. ./build-tags.sh
go build -tags "$(tags_for dev)" -ldflags "$(ldflags_for dev)" -o sui main.go
SUI_DB_FOLDER=db SUI_DEBUG=true ./sui
```

Frontend with hot reload: `cd frontend && npm install && npm run dev`.

### Build tags

`build-tags.sh` is the single source of truth for build tags and the linker flags that go with them. Ask for a profile by name instead of copying a list: `test`, `dev`, `release`, `windows`, `docker`. Without the tags you get stubs for OpenVPN, OpenConnect, Tailscale, Cloudflared and naive instead of the real implementations. CI fails the build if a tag list is pasted anywhere else.

### Environment variables

| Variable | Meaning | Example |
|---|---|---|
| `SUI_DB_FOLDER` | Directory of the SQLite database | `db` |
| `SUI_DEBUG` | Debug mode | `true` |
| `SUI_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `debug` |
| `SUI_BIN_FOLDER` | Directory of the sing-box binaries | `bin` |

### Docker

```bash
docker build -t drnetwork-panel .
# or: docker compose up -d
```

## Conventions

- Standard Go style; run `gofmt -w .` before committing. Handle errors explicitly. Comment what is not obvious - and *why*, not what.
- Layers: `api/` (HTTP handlers, routing) → `service/` (business logic) → `database/model/` (GORM models). Also: `core/` (sing-box), `sub/` (subscriptions), `util/`, `network/`, `service/tgbot/` (Telegram bot). Keep dependencies pointing downwards.
- Panel and nodes talk through the token-authenticated API v2 (`api/apiV2Handler.go`). A new action must be read-only or authenticated like the others, and a master has to cope with a node that does not know it yet (see how `statsTotals` is handled in `service/statsSummary.go`).
- Frontend: user-visible strings go through `$t(...)` and are added to all six locale files (`frontend/src/locales`); other locales fall back to English, but translate anyway.
- Keep functional names intact when changing branding: the `s-ui` command, service and paths, the `s-ui-*` release archives and the `s-ui` session cookie are part of the installer, the updater and existing deployments.

## Testing

Run the tests with the `test` build tags; several protocols sit behind tags and the compatibility tests skip without them:

```bash
. ./build-tags.sh
TAGS=$(tags_for test)
go vet ./...
go test -tags "$TAGS" -ldflags "$(ldflags_for test)" ./...
go test -race -short -tags "$TAGS" -ldflags "$(ldflags_for test)" ./api/... ./service/... ./sub/... ./database/... ./util/... ./logger/... ./network/...
```

Use `test`, not `dev`: `dev` adds the cronet tags, which need a toolchain only the release workflow sets up.

Frontend checks (run in `frontend/`):

```bash
npm run lint -- --max-warnings=0
npx vue-tsc --noEmit
npm test
npm run build
```

Writing tests:

- The standard `testing` package only: `t.Fatal` for setup failures, `t.Errorf` for assertions, table-driven subtests with `t.Run`.
- `t.TempDir()` for scratch space. Service tests call `database.InitDB(...)` for a fully migrated database; node and Telegram tests use `httptest` servers instead of real services.
- Say why a case exists. A regression test names the bug it locks down, and should fail without the fix.
- Touching `api/session.go`, `web/` or `network/`? Check login, a refresh and logout over **both plain HTTP and HTTPS**.

## Pull requests

1. Fork, then branch from `main` (`fix/short-name`, `feature/short-name`).
2. Keep the change focused; avoid unrelated formatting or refactors in the same PR.
3. Make sure formatting, `go vet`, the tests and the frontend checks above pass - CI (`.github/workflows/test.yml`) runs them on every pull request.
4. Open the PR against `main`: say what problem it solves, what changed and how to verify it, and reference the issue (`Fixes #123`).
5. Address review comments with new commits on the same branch.

## Releases

Maintainers only. A release is a bump of the DrNetwork version in `config/release` plus a tag named `v<version>` (for example `v32`). `config/version` is not the panel's version: it follows the S-UI release whose database layout the code uses, and the migrations count by it. Pushing the tag makes the *Release DrNetwork* workflow build the archives for every platform and publish them with the installer. Changes from the S-UI project are imported through the validated synchronisation workflow described in [UPSTREAM_SYNC.md](UPSTREAM_SYNC.md).

## Reporting bugs and requesting features

Use the issue templates in [`.github/ISSUE_TEMPLATE`](.github/ISSUE_TEMPLATE) on [GitHub Issues](https://github.com/Danialrostamani/drnetwork-panel/issues):

- **Bugs** - version, OS, steps to reproduce, expected and actual behaviour.
- **Features** - the use case and, if you have one, a proposed approach.
- **Questions** - the question template.

For a larger feature, open an issue first to agree on the approach.

## Credits

DrNetwork Panel is built on [S-UI](https://github.com/alireza0/s-ui) by Alireza (alireza0) and on [sing-box](https://github.com/SagerNet/sing-box). Thank you both. The project is licensed under GPL-3.0; contributions are accepted under the same license.
