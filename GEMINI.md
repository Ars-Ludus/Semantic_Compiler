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
