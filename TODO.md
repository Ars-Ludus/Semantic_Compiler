# semcom — Open Work

## Correctness

- [ ] **slog migration**: `semcom_orchestrator` still uses `log.Printf`. Migrate to `log/slog` per SPECS §1.

## Features

- [ ] **Direct query subcommand**: `semcom get-context word1 word2 ...` with a 50-point budget, separate from the automated Chat retrieval path.

## Completed

- [x] Personalized Compiler (personal token registry, matcher)
- [x] Grounding framework (EDGES.md, COMPONENTS.md, docs/grounding/)
- [x] Remove semcom_cortex and consolidation.go
- [x] Remove autoRefresh from semcom_retrieve; explicit Refresh after Insert
- [x] Fix MarkMemoryDiscovered ordering (called last in discovery pass)
- [x] Fix GetToken error handling (distinguish ErrNoRows from real errors)
- [x] Fix MaxTurnID vs MaxID in distillation pass (added MaxID to Store)
- [x] WAL mode on shared personalization DB
- [x] Tiered retrieval: 100-pt budget, distilled first (10 pts, 5× personal weight), raw fallback (20 pts), min score 3
- [x] In-memory roaring bitmap retrievers for distillations and personal tokens (DistillationRetriever)
- [x] Phrase-aware tokenization in semcom_embed and semcom_personal (longest-match forward scan)
- [x] Combined distill+discovery pass: single LLM call per chunk returns distillations + entities
- [x] Session ingestion: `ingest-sessions` subcommand for OpenClaw JSONL history
- [x] int32 IDs: narrowed all ID types from int64 to int32 across all modules
- [x] Personal token linking on raw memories: memory_personal_tokens junction table, PersonalRetriever (roaring bitmap), linked on all ingestion paths (Chat, Ingest, distillation pass) with embed+personal match running concurrently
