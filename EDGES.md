# Project Edges (Dependency Map)

Read this file to understand the connections between code files, libraries, and resources. Update this file whenever you modify cross-library interactions.

## Orchestration Layer: semcom_orchestrator
The primary bridge between isolated libraries.

### Edges
- **Store Interface**: `semcom_orchestrator` -> `semcom_store` (via `github.com/ars/semantic_store`)
  - File: `semcom_orchestrator/orchestrator.go`
  - Usage: Persistent memory storage and retrieval.
- **Embedding/Indexing**: `semcom_orchestrator` -> `semcom_embed`
  - File: `semcom_orchestrator/orchestrator.go`
  - Usage: Global vocabulary filtering and vector queries.
- **Personal Registry**: `semcom_orchestrator` -> `semcom_personal`
  - File: `semcom_orchestrator/orchestrator.go`, `semcom_orchestrator/distillation.go`
  - Usage: Personal token matching (Matcher), memory→token linking (Store.LinkMemory, memory_personal_tokens), and personal reverse index for retrieval (PersonalRetriever).
- **Distillation**: `semcom_orchestrator` -> `semcom_distill`
  - File: `semcom_orchestrator/orchestrator.go`, `semcom_orchestrator/distillation.go`
  - Usage: Raw message processing.
- **Retrieval**: `semcom_orchestrator` -> `semcom_retrieve`
  - File: `semcom_orchestrator/orchestrator.go`
  - Usage: Contextual data fetching.
- **LLM Integration**: `semcom_orchestrator` -> `semcom_llm`
  - File: `semcom_orchestrator/main.go`
  - Usage: Providing LLM capability to personal discovery and distillation.
- **Adapter**: `semcom_orchestrator` -> `semcom_adapter`
  - File: `semcom_orchestrator/main.go`
  - Usage: HTTP handler construction. The orchestrator provides a `Dispatcher` closure to `adapter.NewHandler`; the adapter owns request decoding, validation, and response encoding. The openClaw harness (`semcom_adapter/openclaw`) translates the openClaw Plugin SDK wire format.

> For deeper technical details on orchestrator components, see `semcom_orchestrator/COMPONENTS.md`.

## Retrieval Layer: semcom_retrieve
Contextual data fetching services.

### Edges
- **Retrieve-Store Dependency**: `semcom_retrieve` -> `semcom_store`
  - File: `semcom_retrieve/retriever.go`
  - Usage: Full index build on open (`AllSemKeys`) and incremental refresh (`SemKeysSince`). This is a deliberate exception to the isolation rule — the retriever exists solely to index the store.

## Workspace Structure
Managed via `go.work`.
- `/semcom_adapter`: Harness translation layer (canonical types + per-harness adapters).
- `/semcom_embed`: Core vector indexer.
- `/semcom_store`: SQLite-backed memory store.
- `/semcom_personal`: Personal token registry.
- `/semcom_orchestrator`: Main integration service.
- `/semcom_distill`: Raw message processing.
- `/semcom_retrieve`: Contextual data fetching.
- `/semcom_llm`: LLM integration layer.
- `/dashboard`: Web-based visualization.
- `/internal`: Shared utilities and environment configuration.
