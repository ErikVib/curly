# curly

> A fast, lightweight CLI tool to generate and execute curl commands from OpenAPI specifications.

Transform your OpenAPI specs into ready-to-use curl scripts with environment support, making API testing and exploration a breeze.

[![Tests](https://github.com/ErikVib/curly/workflows/Tests/badge.svg)](https://github.com/ErikVib/curly/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/ErikVib/curly)](https://goreportcard.com/report/github.com/ErikVib/curly)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Features

✨ **Generate curl scripts from OpenAPI specs** - Automatically create `.curl` files for each endpoint  
🌍 **Environment management** - Switch between dev, staging, and prod with a single flag  
🔄 **Repeat & parallel execution** - Run requests multiple times for simple load testing or data seeding  
📊 **Built-in statistics** - Track success/failure rates, response times, and throughput with verbose mode 
⚡ **Interactive mode** - Edit requests in your favorite editor before execution  
🎯 **Simple & focused** - Tries to do one thing well, composes with other Unix tools

## Quick Start

### Installation

```bash
# Using Go
go install github.com/ErikVib/curly@latest

# Or build from source
git clone https://github.com/ErikVib/curly.git
cd curly
make build
```

### Basic Usage

```bash
# Generate collection from OpenAPI spec
curly generate openapi.yml

# Or from a URL
curly generate http://localhost:8080/v3/api-docs

# Run a request interactively
curly

# Run a specific request (non-interactively)
curly -f collection/GET_users.curl

# Run with environment variables
curly -e dev 
# or
curly -e dev -f collection/GET_users.curl
```

## Usage

### Generate Collection

Create `.curl` files from an OpenAPI specification:

```bash
curly generate openapi.yml
```

This creates a `collection/` directory with:
- One `.curl` file per endpoint
- An `envs.yml` for environment management
- Variables extracted from path params, query params, and headers

**Example generated file:**
```bash
# GET /users/{id}

# Variables
BASE_URL="http://localhost:8080"
ID="VALUE"
AUTHORIZATION="VALUE"

curl -s -X GET "${BASE_URL}/users/${ID}" \
  -H 'Accept: application/json' \
  -H 'Authorization: ${AUTHORIZATION}'
```

### Interactive Execution

Launch fuzzy finder to select and run a request:

```bash
curly                    # From current directory
curly collection/        # From specific directory
curly -e dev            # With environment
```

This will:
1. Use `fzf` to select a `.curl` file (or fallback to numbered menu)
2. Open it in your `$EDITOR` (defaults to `vim`)
3. Execute the curl command when you save and [quit](https://stackoverflow.com/questions/11828270/how-do-i-exit-vim)

### Direct Execution

Run a specific file without opening the editor:

```bash
curly -f collection/GET_users.curl
curly -e dev -f collection/POST_users.curl
```

### Repeat & Parallel Execution

Perfect for simple load testing or data seeding:

```bash
# Run 10 times sequentially
curly -f api.curl -n 10

# Run 100 times, 10 concurrent at a time
curly -f api.curl -n 100 -p 10

# With verbose output showing progress
curly -f api.curl -n 100 -p 10 -v

# With delay between batches
curly -f api.curl -n 1000 -p 50 --delay=1
```

**Example output:**
```
Running 100 requests (10 concurrent per batch)...
<response outputs...>
Progress: 30/100 (30.0%)
<response outputs...>
Progress: 60/100 (60.0%)
<response outputs...>
Progress: 100/100 (100.0%)

Summary:
  Total:      100
  Success:    98
  Failed:     2
  Duration:   8.5s
  Avg time:   85ms
  Throughput: 11.76 req/s

Errors:
  [2x] command exited with error: connection refused
  [1x] command exited with error: timeout
```

### Environment Management

Define environments in `collection/envs.yml`:

```yaml
environments:
  dev:
    BASE_URL: "http://localhost:8080"
    AUTHORIZATION: "$(curly -e dev -f get-token.curl)"
    USER_ID: "$(uuidgen)"
    USER_NAME: "John Doe"
  prod:
    BASE_URL: "https://api.example.com"
    AUTHORIZATION: "$(curly -e prod -f get-token.curl)"
    USER_ID: "fixed-prod-user-id"
    USER_NAME: "Jane Smith"
```

Use with `-e` flag:

```bash
curly -e dev -f collection/POST_users.curl
curly -e prod -f collection/GET_users.curl -n 10
```

## Command Reference

### `curly generate <openapi-file-or-url>`

Generate `.curl` files from OpenAPI specification.

**Arguments:**
- `<openapi-file-or-url>` - Path to OpenAPI YAML/JSON file or HTTP(S) URL

**Examples:**
```bash
curly generate openapi.yml
curly generate https://petstore3.swagger.io/api/v3/openapi.json
curly generate http://localhost:8080/v3/api-docs
```

### `curly [collection-dir]`

Launch interactive mode to select and run a request.

**Arguments:**
- `[collection-dir]` - Directory containing `.curl` files (default: current directory)

**Flags:**
- `-e, --env <name>` - Environment to use from `envs.yml`
- `-f, --file <path>` - Run specific file without editor
- `-n, --times <N>` - Number of times to execute (default: 1)
- `-p, --parallel <N>` - Number of concurrent executions (default: 1)
- `--delay <seconds>` - Delay between batches in seconds
- `-v, --verbose` - Show progress and detailed output

**Examples:**
```bash
curly
curly collection/
curly -e dev
curly -f api.curl -n 100 -p 10 -v
```

### `curly completion [bash|zsh|fish]`

Generate shell completion script.

**Examples:**
```bash
# Bash
curly completion bash > /etc/bash_completion.d/curly

# Zsh
curly completion zsh > "${fpath[1]}/_curly"

# Fish
curly completion fish > ~/.config/fish/completions/curly.fish
```

## Use Cases

### API Testing & Exploration

```bash
# Generate collection from API docs
curly generate http://localhost:8080/v3/api-docs

# Explore endpoints interactively
curly

# Test with different environments
curly -e staging -f collection/GET_status.curl
```

### Load Testing

```bash
# Simple load test
curly -f api.curl -n 1000 -p 50 -v

# With rate limiting
curly -f api.curl -n 5000 -p 100 --delay=1 -v

# Stress test (all at once)
curly -f api.curl -n 100 -p 100
```

### Data Seeding

```bash
# Create 100 test users
curly -f create-user.curl -n 100

# Each request gets fresh variable evaluation
# e.g., USER_ID=$(uuidgen) generates new UUID for each request
```

### CI/CD Integration

```bash
# Health check with fail-fast
curly -e prod -f health-check.curl -n 5 --delay=5

# Smoke tests or integration tests 
for file in collection/smoke/*.curl; do
  curly -e staging -f "$file" || exit 1
done
```

## Dynamic Variables

Variables in `.curl` files are evaluated by the shell, giving you full flexibility:

```bash
# Variables section
USER_ID=$(uuidgen)
TIMESTAMP=$(date +%s)
TOKEN=$(curl -s http://auth.local/token | jq -r .token)
RANDOM_EMAIL="user-${RANDOM}@example.com"

curl -X POST "${BASE_URL}/users" \
  -d "{\"id\":\"${USER_ID}\",\"email\":\"${RANDOM_EMAIL}\"}"
```

Each execution gets fresh values - perfect for load testing or generating test unique data.

## Configuration

### Environment Variables

- `EDITOR` - Editor to use in interactive mode (default: `vim`)

### Files

- `collection/` - Generated `.curl` files
- `collection/envs.yml` - Environment configurations

## Requirements

- Go 1.20+ (for building from source)
- `fzf` (optional, for fuzzy finding)
- An editor set in `$EDITOR` (defaults to `vim`)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request or an Issue.

### Development Setup

```bash
git clone https://github.com/ErikVib/curly.git
cd curly
go mod download
make build
```

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with [cobra](https://github.com/spf13/cobra) for CLI framework
- Uses [kin-openapi](https://github.com/getkin/kin-openapi) for OpenAPI parsing
- Inspired by the Unix philosophy: do one thing well

## Support

- 🐛 [Issue Tracker](https://github.com/ErikVib/curly/issues)
- 💬 [Discussions](https://github.com/ErikVib/curly/discussions)

---
