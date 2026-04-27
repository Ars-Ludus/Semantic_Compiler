# Grounding Framework Design (Spec)

## 1. Overview
A three-tier system to provide persistent identity, operational mapping, and technical grounding for the Gemini CLI agent. This system aims to maintain high quality, reliability, and efficiency while strictly adhering to the "Isolated Library" architecture of the `semcom` project.

## 2. Components

### A. `GEMINI.md` (The Persistent Identity)
Located at the root. Acts as a secondary system prompt.
- **Architecture Mandate**: Libraries (`semcom_*`) MUST be isolated. No direct cross-library calls. All orchestration happens in `semcom_orchestrator` or `conductor`.
- **Error Handling**: Use `log/slog`. Prefer structured logging over `fmt.Errorf`.
- **The "Edge" Protocol**: Before searching or grepping, read `EDGES.md` to understand the landscape.
- **Self-Evolution**: If I detect a deficiency in my rules or system prompt that hinders quality or reliability, I must flag it for discussion.

### B. `EDGES.md` (The Operational Map)
Located at the root. A living document of connections.
- **Structure**: Grouped by "Library" and "Interaction Flow".
- **Content**: File paths, interface implementations, and key call sites.
- **Maintenance**: Updated by the agent whenever an interaction between isolated components is added, changed, or removed.

### C. `docs/grounding/` (The Technical Anchor)
A directory for distilled external knowledge.
- **Purpose**: To compensate for knowledge cutoffs and provide stable APIs for libraries.
- **Process**: Research via `web_fetch`/`context7`, distill to a `.md` file, and refer to it in future turns.

## 3. Implementation Plan
1. Create `GEMINI.md` with core mandates.
2. Create `EDGES.md` by analyzing current `semcom_orchestrator` and `conductor` interactions.
3. Establish `docs/grounding/` directory.

## 4. Success Criteria
- The agent can locate relevant code for an orchestration task without exhaustive `grep`.
- No direct dependencies are introduced between `semcom_embed`, `semcom_store`, and `semcom_personal`.
- `slog` usage remains consistent across the workspace.
