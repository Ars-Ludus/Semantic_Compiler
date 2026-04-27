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
  - File: `semcom_orchestrator/orchestrator.go`
  - Usage: Token discovery and memory matching.
- **Distillation**: `semcom_orchestrator` -> `semcom_distill`
  - File: `semcom_orchestrator/orchestrator.go`, `semcom_orchestrator/distillation.go`
  - Usage: Raw message processing.
- **Retrieval**: `semcom_orchestrator` -> `semcom_retrieve`
  - File: `semcom_orchestrator/orchestrator.go`
  - Usage: Contextual data fetching.

> For deeper technical details on orchestrator components, see `semcom_orchestrator/COMPONENTS.md`.

## Workspace Structure
Managed via `go.work`.
- `/semcom_embed`: Core vector indexer.
- `/semcom_store`: SQLite-backed memory store.
- `/semcom_personal`: Personal token registry.
- `/semcom_orchestrator`: Main integration service.
- `/semcom_distill`: Raw message processing.
- `/semcom_retrieve`: Contextual data fetching.
- `/semcom_llm`: LLM integration layer.
- `/dashboard`: Web-based visualization.
- `/internal`: Shared utilities and environment configuration.
