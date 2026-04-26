# Personalized Compiler Design Specification

## Background & Motivation
The existing `semcom` system acts as a "Global Compiler," mapping 73k static tokens to a 4-level deterministic semantic hierarchy (`l0_ids`), enabling sub-1ms retrieval. However, it lacks the ability to process and retain Out-of-Vocabulary (OOV) terms representing personal entities, names, locations, or events unique to the user. 

To solve this, we are building a "Personalized Compiler" as a high-speed partner to the global system. This partner will adaptively learn user-specific entities through a background LLM discovery process while maintaining the sub-millisecond retrieval guarantees of the main pipeline.

## Scope & Architecture
The system relies on a Dual-Compiler Architecture. We will create a consolidated Go library, `semcom_personal`, to handle the storage, in-memory matching, and background discovery of personal tokens.

### 1. `semcom_personal` Library Components
- **Matcher**: An in-memory, high-speed lookup mechanism (e.g., Trie or Map) initialized from SQLite. It evaluates OOV words, converting known words into `personal_ids` and dropping known noise via an Ignore List.
- **Registry**: A SQLite-backed persistent store containing three primary structures:
  - `personal_tokens`: Defines the learned entities (`id`, `word`, `type` [Personal L0]).
  - `personal_ignore`: Defines words explicitly marked as noise by the LLM or system.
  - `personal_semkeys`: Maps `personal_id` to `memory_id` for fast reverse-index retrieval.
- **Discoverer**: An asynchronous background service. It consumes batches of unmapped OOV words and surrounding context, querying an LLM with a strict JSON schema to classify new entities and identify noise.

### 2. Orchestrator Modifications
- **Dual-Stream Tokenization**: The Orchestrator's main loop will execute parallel calls:
  - Global Stream: `semcom_embed.Query(text)` -> returns `global_l0_ids` and `oov_words`.
  - Personal Stream: `semcom_personal.Match(oov_words)` -> returns `personal_ids` (known entities) and `unmapped_words` (sent to Discovery).
- **Merging**: The Orchestrator merges `global_l0_ids` and `personal_ids` for retrieval, adding zero wall-clock latency to the pipeline.

### 3. `semcom_store` Modifications
- **Memories Table**: Add a new `personal_tokens` column to the `memories` table to record the array of `personal_ids` associated with each message natively, simplifying future relational graph generation.

## Data Flow
1. **Ingest/Chat**: Input is received. `semcom_embed` tokenizes the input, extracting `l0_ids` and a list of OOV words.
2. **Local Match & Filter**: `semcom_personal` evaluates OOV words. 
   - Known entities -> mapped to `personal_id`.
   - Known noise -> ignored.
   - Unknown -> placed in the Discovery queue.
3. **Retrieval & Storage**: The Orchestrator retrieves memories using both global and personal IDs. It stores the new memory, linking both ID sets in the database.
4. **Background Discovery**: The Discoverer prompts an LLM with the unknown words and context. 
   - Identified entities are saved to `personal_tokens`.
   - Non-entities are saved to `personal_ignore`.
   - The in-memory Matcher is updated atomically.

### 5. Backfill & Retroactive Tagging
To process existing chat history and extract personal semkeys, the system employs a two-pronged strategy:
1. **Initial Backfill Pass**: A `Backfill()` method in the Orchestrator iterates over all `memories` where `personal_tokens IS NULL`. It identifies already-known personal entities (updating the DB immediately) and pushes unknown words to the `unmappedCh` queue for discovery. It then marks the memory as processed to prevent infinite loops.
2. **Worker Retroactive Scan**: The Background Worker is provided access to the semantic store. Whenever the LLM discovers a new entity, the worker queries the database for all historical memories containing that exact word. It parses them to ensure a strict token match and appends the newly discovered `personal_id` to both the `personal_tokens` column and the `memory_semkeys` table.

## Trade-offs & Alternatives Considered
- **Vector Embeddings**: Rejected due to latency and the inability to establish deterministic, discrete relationship nodes.
- **Synchronous Discovery**: Rejected. Querying an LLM inline during a chat turn would break the sub-1ms performance goal.
- **Multiple Disparate Libraries**: Initially considered creating separate store, embed, and retrieve libraries for the personal compiler. Consolidated into a single `semcom_personal` library for simplified synchronization of the Matcher and Registry states.

## Verification & Rollback
- **Tests**: 
  - Ensure Matcher and Filter logic executes in microseconds.
  - Verify JSON unmarshaling and error handling for the LLM discovery prompt.
  - Ensure SQLite transactions atomicly update the Matcher state.
- **Rollback**: Personal tokens can be wiped by clearing the `personal_registry` tables. The global index remains completely unaffected by personal state.
