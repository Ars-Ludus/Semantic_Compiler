# Grounding Framework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish a three-tier grounding framework (Identity, Mapping, Technical Anchors) to improve agent reliability and efficiency.

**Architecture:** 
- `GEMINI.md`: Root-level persistent identity and architectural mandates.
- `EDGES.md`: Root-level dependency map for cross-library interactions.
- `docs/grounding/`: Directory for distilled technical knowledge of external dependencies.

**Tech Stack:** Markdown, Go (for dependency analysis).

---

### Task 1: Initialize GEMINI.md

**Files:**
- Create: `GEMINI.md`

- [ ] **Step 1: Write GEMINI.md content**
Write the following content to `GEMINI.md`:

```markdown
# Agent Identity & Mandates

You are the Gemini CLI agent. You operate within the `semcom` project, a modular system for semantic memory and personal intelligence.

## 1. Architectural Mandates
- **Strict Isolation**: Libraries (`semcom_*`) MUST be treated as independent modules. They must NOT import each other directly.
- **Orchestration Layer**: All cross-library logic resides in `semcom_orchestrator` or `conductor`.
- **Error Handling**: Use `log/slog` for structured logging. Avoid `fmt.Errorf` for operational errors; provide context via slog attributes.

## 2. Efficiency & Navigation
- **Consult EDGES.md**: Before performing broad searches or greps, read `EDGES.md` to understand the connections between code files and libraries.
- **Update EDGES.md**: Whenever you add, edit, or remove code that interacts with another library, update the corresponding edge in `EDGES.md`.
- **Grounding Docs**: Use `docs/grounding/` to store distilled knowledge of external libraries to prevent hallucinations caused by knowledge cutoffs.

## 3. Self-Evolution
- **Feedback Loop**: If you identify a deficiency in this `GEMINI.md` file or your core system prompt that hinders your quality, reliability, or efficiency, you MUST bring it to the user's attention for discussion.
```

- [ ] **Step 2: Commit GEMINI.md**

```bash
git add GEMINI.md
git commit -m "chore: initialize GEMINI.md agent mandates"
```

---

### Task 2: Analyze and Create EDGES.md

**Files:**
- Create: `EDGES.md`
- Reference: `semcom_orchestrator/orchestrator.go`, `go.work`

- [ ] **Step 1: Analyze orchestrator edges**
Verify dependencies in `semcom_orchestrator/orchestrator.go` and `go.work`.

- [ ] **Step 2: Write EDGES.md content**
Create `EDGES.md` with the following initial map:

```markdown
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
  - File: `semcom_orchestrator/orchestrator.go`
  - Usage: Raw message processing.
- **Retrieval**: `semcom_orchestrator` -> `semcom_retrieve`
  - File: `semcom_orchestrator/orchestrator.go`
  - Usage: Contextual data fetching.

## Workspace Structure
Managed via `go.work`.
- `/semcom_embed`: Core vector indexer.
- `/semcom_store`: SQLite-backed memory store.
- `/semcom_personal`: Personal token registry.
- `/semcom_orchestrator`: Main integration service.
```

- [ ] **Step 3: Commit EDGES.md**

```bash
git add EDGES.md
git commit -m "docs: initialize EDGES.md dependency map"
```

---

### Task 3: Grounding Directory & Initial Grounding

**Files:**
- Create: `docs/grounding/README.md`
- Create: `docs/grounding/go-slog.md`

- [ ] **Step 1: Create grounding directory**
Create `docs/grounding/` and a basic `README.md`.

- [ ] **Step 2: Distill slog grounding**
Since `slog` is a core mandate, create `docs/grounding/go-slog.md` with key patterns found in the codebase.

- [ ] **Step 3: Commit grounding docs**

```bash
git add docs/grounding/
git commit -m "docs: initialize grounding directory and slog anchor"
```
