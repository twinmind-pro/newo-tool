# newo-tool

A command-line interface (CLI) for interacting with the newo.ai platform.

## Installation

### Homebrew (macOS and Linux)

```bash
brew tap twinmind/newo
brew install newo
```

### Manual

1. Download the latest release from the [GitHub Releases](https://github.com/twinmind/newo-tool/releases) page.
2. Unpack the archive.
3. Move the `newo` binary to a directory in your `$PATH` (e.g., `/usr/local/bin`).

## Configuration

The `newo` CLI can be configured via a `newo.toml` file in the current directory or via environment variables.

### `newo.toml` file

The `newo.toml` file contains sensitive information (API keys) and should not be committed to version control. This repository contains a `newo.toml.example` file that you can use as a template.

1. Copy `newo.toml.example` to `newo.toml`:
   ```bash
   cp newo.toml.example newo.toml
   ```
2. Edit `newo.toml` to add your API keys and other settings.

```toml
[defaults]
# Path to download projects to. If empty, downloads to the current directory.
output_root = ""

# A prefix to be added to the project slug (directory name).
slug_prefix = ""

# The base URL of the NEWO platform.
base_url = "https://app.newo.ai"

# The default customer IDN to use if not specified otherwise.
default_customer = ""

# The default project UUID to use.
project_id = ""

# You can define multiple customer profiles.
[[customers]]
idn = "customer-idn-1"
api_key = "your-api-key-1"
project_id = "project-uuid-1"

[[customers]]
idn = "customer-idn-2"
api_key = "your-api-key-2"
project_id = "project-uuid-2"
```

### Environment Variables

You can also use environment variables to configure the CLI. These will override the values in `newo.toml`.

- `NEWO_BASE_URL`: The base URL of the NEWO platform.
- `NEWO_PROJECT_ID`: The default project UUID.
- `NEWO_API_KEY`: An API key for a single customer.
- `NEWO_DEFAULT_CUSTOMER`: The default customer IDN.
- `NEWO_OUTPUT_ROOT`: Path to download projects to.

## Usage

### `newo help`

Show usage information for a command.

```bash
newo help [command]
```

### `newo version`

Show build version and commit.

```bash
newo version
```

### `newo pull`

Synchronise projects, agents, flows, and skills from NEWO to disk.

```bash
newo pull [flags]
```

**Flags:**

- `--force`: Overwrite local skill scripts without prompting.
- `--verbose`: Enable verbose logging.
- `--customer <idn>`: Customer IDN to limit the pull to.
- `--project <uuid>`: Restrict pull to a single project UUID.

### `newo push`

Upload local changes back to NEWO.

```bash
newo push [flags]
```

**Flags:**

- `--verbose`: Show detailed output.
- `--customer <idn>`: Customer IDN to push.
- `--no-publish`: Skip publishing flows after upload.
- `--force`: Skip interactive diff and confirmation.

### `newo status`

Show local changes since the last pull.

```bash
newo status [flags]
```

**Flags:**

- `--verbose`: Show detailed information.
- `--customer <idn>`: Customer IDN to inspect.

### `newo lint`

Lint `.nsl` files in downloaded projects.

```bash
newo lint
```

### `newo fmt`

Format `.nsl` files in downloaded projects. This command will automatically trim trailing whitespace and collapse multiple blank lines to a single blank line.

```bash
newo fmt
```

## Development

This project uses Go modules and a `Makefile` for common tasks.

### Build

To build the `newo` binary:

```bash
make build
```

### Test

To run the tests:

```bash
make test
```

### Lint

To run the linter:

```bash
make lint
```

### Format

To format the code:

```bash
make fmt
```
