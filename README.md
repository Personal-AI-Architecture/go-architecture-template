# Go PAA Template

Reference Go implementation for the [Personal AI Architecture (PAA)](https://github.com/Personal-AI-Architecture/the-architecture).
It is designed to help you understand PAA boundaries quickly, run a working
local example without a provider account, and extend the implementation without
losing contract alignment.

## What you get

- A canonical PAA Gateway with conversation routes and SSE responses.
- A provider-neutral Agent Loop and deterministic offline mock adapter.
- A clear Go adapter boundary for connecting a real model provider.
- Runtime configuration that keeps provider selection out of request metadata.
- Pinned and vendored PAA contracts under `paa.lock.json`, `specs/`, and
  `vendor/paa-contracts/`.
- Go-native contract, import-boundary, lock-in, formatting, vet, unit,
  integration, conformance, and acceptance checks.
- Integration with the shared language-neutral PAA conformance validator.

The starter uses Go's standard library for its runtime and demo server. It has
no third-party runtime dependencies.

## Requirements

- Go 1.22 or newer
- `curl` for the HTTP walkthrough
- Node.js 20 or newer only when running the shared cross-language validator

## Quickstart

The first demo requires no model account or network connection:

```bash
go run ./cmd/demo
```

You should see a request enter the Gateway, pass through the Agent Loop and
mock adapter, emit `text-delta` and `done` events, and persist a two-message
conversation.

## Run the HTTP example

Start the local server:

```bash
go run ./cmd/server
```

In another terminal, check the demo host and send a canonical PAA message:

```bash
curl http://127.0.0.1:8000/health
curl -N -X POST http://127.0.0.1:8000/conversations/messages -H 'Content-Type: application/json' -H 'X-Actor-ID: demo-user' -H 'X-Actor-Permissions: memory:read,memory:write' -d '{"content":"Explain what PAA is."}'
```

The response is a Server-Sent Events stream. `/health` belongs to the local
demo host; the conversation routes are the PAA-facing contract.

## Validate the template

Run the complete self-contained Go validation path:

```bash
go run ./cmd/validate
```

This runs contract and schema checks, import-boundary and provider lock-in
checks, `gofmt`, `go vet`, all Go tests, and the acceptance flow.

To compare the vendored contracts with the exact PAA source commit and run the
shared external gate as well:

```bash
go run ./cmd/validate \
  -source-checkout /path/to/personal-ai-architecture \
  -shared-validator /path/to/paa-conformance-validator/bin/paa-validate.js
```

The current contract baseline is pinned to:

```text
9565154901e57f184bfdd3cef5f23dd4014e2c03
```

The Go checks are the internal implementation gate. The shared validator is
the external, language-neutral gate; both should pass before publishing a
template change.

The included GitHub Actions workflow runs the same checks against the exact
pinned PAA source on every push and pull request.

## PAA request flow

```text
Client
  -> Gateway routes
  -> canonical EngineRequest
  -> Agent Loop
  -> Model Adapter
  -> canonical stream events
  -> SSE response and conversation memory
```

Provider-native payloads stay inside `internal/adapters/`. The Agent Loop and
Gateway operate on canonical PAA message, tool, metadata, and stream shapes.

## Routes

| Method | Route | Purpose |
|---|---|---|
| `POST` | `/conversations/messages` | Start a conversation and stream a response |
| `GET` | `/conversations` | List conversation summaries |
| `GET` | `/conversations/{id}` | Read conversation history |
| `POST` | `/conversations/{id}/messages` | Continue an existing conversation |
| `POST` | `/chat` | Compatibility alias documented as `EXT-001` |
| `GET` | `/health` | Local demo-server health check; not a PAA contract route |

Message requests use the PAA public shape:

```json
{
  "content": "Help me plan my finances",
  "metadata": {
    "source": "example-client"
  }
}
```

The Gateway requires `X-Actor-ID` and `X-Actor-Permissions` for message
requests. The demo uses simple headers so the boundary is visible; production
authentication and authorization should be supplied by the host application.

## Adapters and providers

`MockAdapter` is deterministic and offline, making the starter safe to run in
CI and in a first local walkthrough. `OpenAICompatibleAdapter` demonstrates
where provider-native `/chat/completions` transport belongs. It accepts an
injected HTTP client and does not require credentials or network access in the
template.

To add a provider:

1. Implement the adapter under `internal/adapters/`.
2. Translate provider payloads into canonical PAA types at that boundary.
3. Register runtime selection in `internal/adapters/loader.go`.
4. Add adapter behavior tests and run `go run ./cmd/validate`.

Do not put provider URLs, API keys, model overrides, or provider-shaped tool
payloads into the Gateway or Agent Loop.

## Project layout

```text
internal/
  adapters/       Provider boundary and adapter loader
  checks/         Contract, import, formatting, and lock-in checks
  config/         Runtime configuration
  engine/         Provider-neutral Agent Loop
  gateway/        PAA routes, conversation store, and HTTP host
  memory/         Memory persistence boundary
  types/          Canonical PAA contracts
cmd/
  demo/           In-process PAA walkthrough
  server/         Local HTTP/SSE demo host
  validate/       Complete local and optional external validation
test/
  integration/    Gateway lifecycle and HTTP tests
  conformance/    Contract and boundary tests
specs/            Local PAA contract mirror
vendor/           Exact pinned PAA contract source
```

## Scope and production notes

This is a reference starter, not a production deployment. The default memory
store is in-process, the mock adapter is intentionally simple, and the
standard-library HTTP host is for demonstration. A production host should add
persistent storage, authentication middleware, operational logging, provider
credentials management, and deployment-specific controls while preserving the
PAA contract boundary.
