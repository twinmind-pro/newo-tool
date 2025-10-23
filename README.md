# newo-tool

`newo` is the CLI companion for the NEWO platform. Pull projects to disk, lint and format NSL scripts, run health checks, merge environments, generate code via LLMs, and push your changes back safely.

---
## Installation

### Homebrew (macOS & Linux)
```bash
brew tap twinmind/newo
brew install newo
```
Upgrade when a new release ships:
```bash
brew upgrade newo
```

### Manual download
1. Download the latest release from the [GitHub Releases](https://github.com/twinmind/newo-tool/releases) page.
2. Unpack the archive for your platform.
3. Move the `newo` binary somewhere in your `$PATH` (for example `/usr/local/bin`).

### Build from source
```bash
make build
./bin/newo version
```
Requires Go 1.21+.

---
## Quick start
1. Place a `newo.toml` in your workspace (see example below).
2. Pull: `newo pull --customer calcom`.
3. Edit local `.nsl` / `.meta.yaml` files.
4. Lint & optionally fix: `newo lint --customer calcom --fix`.
5. Push: `newo push --customer calcom` (publishes flows unless `--no-publish`).

---
## Configuration

### Example `newo.toml`
```toml
[defaults]
output_root = "integrations"            # local export root for pull/push
slug_prefix = ""
include_hidden_attributes = true
base_url = "https://app.newo.ai"

default_customer = "NEYjADiZWc"

[[customers]]
idn = "NEYjADiZWc"
alias = "calcom"
type = "integration"
api_key = "<api-key>"
  [[customers.projects]]
    idn = "calcom"
  [[customers.projects]]
    idn = "calcom-sandbox"

[[customers]]
idn = "NEvwc3hSaW"
alias = "gohigh-e2e"
type = "e2e"
api_key = "<api-key>"
  [[customers.projects]]
    idn = "gohighlevel"

[[llms]]
provider = "openai"
model = "gpt-4.1-mini"
api_key = "<llm-api-key>"
```
> A customer can declare multiple `[[customers.projects]]` entries; commands like `pull`, `fmt`, and `lint` iterate over each project for the selected customer.

### Environment variables
| Variable | Description |
| --- | --- |
| `NEWO_BASE_URL` | Override the API base URL. |
| `NEWO_OUTPUT_ROOT` | Override export root. |
| `NEWO_PROJECT_ID` / `NEWO_PROJECT_IDN` | Default project identifiers. |
| `NEWO_DEFAULT_CUSTOMER` | Default customer IDN or alias. |
| `NEWO_API_KEY` / `NEWO_API_KEYS` | Provide API keys when TOML is absent. |
| `NEWO_SLUG_PREFIX` | Prefix applied to generated slugs. |
| `NEWO_ACCESS_TOKEN`, `NEWO_REFRESH_TOKEN`, `NEWO_REFRESH_URL` | Optional automatic token refresh. |
| `NO_COLOR` | Disable ANSI colour output. |

Aliases defined in `newo.toml` are accepted everywhere `--customer` is used.

---
## Commands

### `newo help [command]`
Show usage information.

### `newo version`
Print build metadata.

### `newo pull`
Synchronise projects, agents, flows, and skills from NEWO to disk.
```
newo pull [flags]
```
**Flags:** `--customer <idn|alias>`, `--project-uuid <uuid>`, `--project-idn <idn>`, `--force`, `--verbose`.

- Overwrite prompts accept `y` (overwrite this file), `n`/enter (skip), and `a` (apply the overwrite decision to the rest of the run).

### `newo push`
Upload local changes back to NEWO.
```
newo push [flags]
```
**Flags:** `--customer <idn|alias>`, `--no-publish`, `--force`, `--verbose`.

### `newo status`
Compare local state with the last pull.
```
newo status [flags]
```
**Flags:** `--customer <idn|alias>`, `--verbose`.

### `newo lint`
Run NSL linting.
```
newo lint [flags]
```
**Flags:** `--customer <idn|alias>`, `--fix`. With `--fix` the CLI interactively removes NSL `{# … #}` comments (answers: `y` apply once, `n` skip, `a` apply to the rest).

### `newo fmt`
Format `.nsl` files (trim trailing whitespace, collapse extra blank lines).
```
newo fmt [flags]
```
**Flags:** `--customer <idn|alias>`.

### `newo generate`
Generate NSL snippet via the configured LLM.
```
newo generate --prompt "Describe the logic"
```
Requires at least one `[[llms]]` entry in `newo.toml`.

### `newo healthcheck`
Run environment/configuration checks (config, filesystem, connectivity, local state, external tools).
```
newo healthcheck
```

### `newo merge`
Copy an e2e customer’s project into an integration customer.
```
newo merge <project_idn> from <source_customer_idn> [flags]
```
**Flags:** `--target-customer <idn|alias>`, `--no-pull`, `--no-push`, `--force`.

---
## Development workflow
| Command | Description |
| --- | --- |
| `make build` | Build `./bin/newo`. |
| `make test` | Run Go tests. |
| `make lint` | Run `golangci-lint`. |
| `make fmt` | Apply `gofmt`. |

---
## Tips
- Use customer aliases to keep commands short: `newo pull --customer calcom`.
- `newo lint --fix` currently targets NSL comments; more fixers will be added over time.
- Set `NO_COLOR=1` when piping output into tools that cannot handle ANSI colours.
