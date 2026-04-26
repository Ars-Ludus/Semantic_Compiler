# Implementation Plan: Chat History Backfill & Discovery Worker Retroactive Scan

Process existing chat history to retroactively identify and tag personal entities, filling in the `personal_tokens` column and `memory_semkeys` table.

## User Review Required

> [!IMPORTANT]
> This plan assumes that the `personal_tokens` column in `semcom_store` can be initialized to `NULL` for unprocessed messages and `'[]'` for messages that have been processed but contain no personal entities.

- **Storage Method**: Should we use `'[]'` to mark a message as "processed but empty"?
  - *Decision*: Yes, to distinguish from `NULL` (unprocessed).

## Proposed Changes

### 1. `semcom_store` (Storage Layer)

- **New Methods in `Store` Interface**:
  - `MemoriesWithoutPersonalTokens(ctx context.Context) ([]*Memory, error)`: Fetches memories where `personal_tokens IS NULL`.
  - `MemoriesContainingWord(ctx context.Context, word string) ([]*Memory, error)`: Uses a `LIKE` query to find candidate memories for a newly learned word.
  - `UpdateMemoryPersonalTokens(ctx context.Context, memoryID int64, personalIDs []uint32) error`: Updates the `personal_tokens` column and inserts into `memory_semkeys` for each ID.

### 2. `semcom_orchestrator` (Orchestration Layer)

- **Backfill Method**:
  - Add `Backfill(ctx context.Context)` to `Orchestrator`.
  - Fetches unprocessed memories.
  - Runs `embedAndMatch(raw)`.
  - Updates the database with any immediate matches.
  - Queues unknowns to `unmappedCh`.
  - Marks memory as processed (`[]`) if it was the first time seeing it.
- **Discovery Worker Update**:
  - Update `startDiscoveryWorker` to accept a `Store` (from `semcom_store`).
  - After `store.InsertToken` (personal registry), call a new helper `retroactiveTag(word, newID)`.
  - `retroactiveTag` fetches candidate memories, verifies word boundaries, and updates `semcom_store`.

### 3. CLI & Entry Point

- Add a `-backfill` flag to `semcom_orchestrator/main.go` to trigger the process on startup.

## Verification Plan

### Automated Tests
- **Integration Test**: 
  1. Insert a memory with "Hello Alice" into a clean `semcom_store`.
  2. Start `Orchestrator` with `MockLLM`.
  3. Call `Backfill()`.
  4. Verify that "Alice" is discovered and the memory row is updated with the personal ID for "Alice".
  5. Verify `memory_semkeys` contains the new ID.

### Manual Verification
- Run `semcom -backfill` on the existing project database and inspect the `personal_tokens` column in SQLite.
