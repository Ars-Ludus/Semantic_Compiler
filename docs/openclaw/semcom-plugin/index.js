import { appendFileSync, mkdirSync } from "fs";
import { homedir } from "os";
import { dirname } from "path";

const SEMCOM_DIR = process.env.SEMCOM_DIR ?? `${homedir()}/.local/share/semcom`;
const SEMCOM_URL = process.env.SEMCOM_URL ?? "http://localhost:8080";
const TIMEOUT_MS = 500;  // semcom responds in <10ms — no need for 3s
const TOP_K = 3;         // 3 is plenty, 5 was overwhelming
const MIN_SCORE = 5;     // ignore low-relevance noise
const LOG_FILE = `${SEMCOM_DIR}/semcom-hooks.log`;

function log(msg) {
  const line = `[${new Date().toISOString()}] [semcom-plugin] ${msg}\n`;
  try {
    mkdirSync(dirname(LOG_FILE), { recursive: true });
    appendFileSync(LOG_FILE, line);
  } catch {}
}

function truncate(str, max) {
  if (!str) return "";
  if (str.length <= max) return str;
  return str.slice(0, max) + "...";
}

async function semcomPost(body) {
  return fetch(`${SEMCOM_URL}/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(TIMEOUT_MS),
  });
}

/**
 * Extract the actual user message from the inbound context envelope.
 * event.prompt looks like:
 *   Sender (untrusted metadata):\n```json\n{...}\n```\n\n[Timestamp] Actual user message
 * We strip the JSON metadata block and grab the actual message.
 */
function extractUserPrompt(raw) {
  if (!raw) return "";
  // Strip the "Sender (untrusted metadata):\n```json\n...\n```\n" prefix
  const stripped = raw.replace(
    /^Sender\s+\(untrusted\s+metadata\):\s*\n```json\n[\s\S]*?\n```\n*/,
    ""
  ).trim();

  // If all that's left is a timestamp prefix, strip that too
  const timestampStripped = stripped.replace(
    /^\[.*?\]\s*/,
    ""
  ).trim();

  return timestampStripped || raw;
}

/**
 * Skip triggers that aren't actual user conversations — heartbeat, cron, system events.
 * These produce noise instead of meaningful memory queries or ingestion.
 */
function isUserTrigger(trigger) {
  if (!trigger) return true; // when in doubt, try
  const skip = new Set(["heartbeat", "cron", "system", "system_event", "reconnect"]);
  return !skip.has(trigger);
}

/**
 * Fire-and-forget ingest: never throw, never block the caller.
 */
function fireAndForgetIngest(prompt, by, event, ctx) {
  const payload = {
    operation: "ingest",
    prompt,
    by,
    session_id: event.sessionId ?? ctx.sessionId,
  };
  semcomPost(payload)
    .then(async (resp) => {
      if (resp.ok) {
        const body = await resp.text();
        log(`ingest (by=${by}) success: HTTP ${resp.status} body=${truncate(body, 100)}`);
      } else {
        log(`ingest (by=${by}) HTTP error: ${resp.status} ${resp.statusText}`);
      }
    })
    .catch((err) => log(`ingest (by=${by}) error: ${err?.message ?? err}`));
}

function formatMemoryBlock(hits) {
  const lines = hits.map((h) => {
    const label = h.topic ? `[${h.type} | ${h.topic}]` : `[${h.type}]`;
    return `${label} ${h.content}`;
  });
  return `<semcom_memory>\n${lines.join("\n")}\n</semcom_memory>`;
}

export default {
  id: "semcom-memory",
  name: "Semcom Memory",
  description: "Injects relevant semantic memories from semcom before each reply; ingests agent responses into semcom's store.",

  register(api) {
    log("plugin registered");

    // ──────────────────────────────────
    // READ: Query semcom for relevant context to inject
    // ──────────────────────────────────
    api.on("before_prompt_build", async (event, ctx) => {
      log(`\n=== BEFORE_PROMPT_BUILD ===`);
      log(`agentId=${ctx.agentId} sessionKey=${ctx.sessionKey} trigger=${ctx.trigger}`);

      // Skip non-user triggers — no point querying semcom for heartbeat boilerplate
      if (!isUserTrigger(ctx.trigger)) {
        log(`skipping — non-user trigger: ${ctx.trigger}`);
        return;
      }

      // Extract the real user query from the metadata envelope
      const rawPrompt = event.prompt;
      const userQuery = extractUserPrompt(rawPrompt);

      if (!userQuery || userQuery.length < 3) {
        log(`skipping — empty or too-short user query after stripping envelope`);
        return;
      }

      log(`raw prompt length=${rawPrompt?.length ?? 0}`);
      log(`extracted user query (first 200): ${truncate(userQuery, 200)}`);

      // ── READ: query semcom for relevant context ──
      try {
        const resp = await semcomPost({
          operation: "chat",
          prompt: userQuery,
          by: "user",
          session_id: ctx.sessionId,
          top_k: TOP_K,
        });
        if (!resp.ok) {
          log(`semcom returned HTTP ${resp.status} ${resp.statusText}`);
          return;
        }
        const data = await resp.json();
        const hits = (data.context ?? []).filter(h => (h.score ?? 0) >= MIN_SCORE);

        if (hits.length === 0) {
          log(`semcom returned 0 hits above score threshold (min=${MIN_SCORE})`);
          return;
        }

        log(`semcom returned ${hits.length} filtered hit(s):`);
        hits.forEach((h, i) => {
          log(`  hit[${i}]: id=${h.id} type=${h.type} topic=${h.topic} score=${h.score} content=${truncate(h.content, 150)}`);
        });

        const memoryBlock = formatMemoryBlock(hits);
        const result = { prependContext: memoryBlock + "\n\n" };

        log(`returning prependContext (${result.prependContext.length} chars):`);
        log(`--- BEGIN PREPEND ---`);
        log(result.prependContext);
        log(`--- END PREPEND ---`);

        return result;
      } catch (err) {
        log(`semcom unreachable or error: ${err?.message ?? err}`);
      }
    });

    // ──────────────────────────────────────────────────
    // WRITE: Ingest the agent's final natural answer
    //
    // Two hooks cover both agent harness types:
    //   - llm_output:  fires for the embedded Pi-agent (once per turn with final text)
    //   - before_agent_finalize: fires for Codex harness (lastAssistantMessage)
    //
    // Whichever fires first ingests the response; the second call is a no-op
    // because lastResponseText is already set for this session+run combination.
    //
    // Excludes heartbeats, cron, system events via trigger filter.
    // ──────────────────────────────────────────────────

    // Track already-ingested responses to avoid duplicates
    const ingestedRuns = new Set();

    api.on("llm_output", (event, ctx) => {
      if (ctx.trigger !== "user") return;

      const text = event.assistantTexts?.join("\n")?.trim();
      if (!text) return;

      const dedupKey = `${ctx.sessionId}:${event.runId}`;
      if (ingestedRuns.has(dedupKey)) return;
      ingestedRuns.add(dedupKey);

      log(`\n=== LLM_OUTPUT ===`);
      log(`sessionKey=${ctx.sessionKey} agentId=${ctx.agentId} sessionId=${event.sessionId}`);
      log(`response length=${text.length} first 200: ${truncate(text, 200)}`);

      fireAndForgetIngest(text, "model", event, ctx);
    });

    api.on("before_agent_finalize", (event, ctx) => {
      if (ctx.trigger !== "user") return;
      if (event.stopHookActive) return;

      const response = event.lastAssistantMessage;
      if (!response) return;

      const dedupKey = `${ctx.sessionId}:${event.runId}`;
      if (ingestedRuns.has(dedupKey)) return;
      ingestedRuns.add(dedupKey);

      log(`\n=== BEFORE_AGENT_FINALIZE ===`);
      log(`sessionKey=${ctx.sessionKey} agentId=${ctx.agentId} sessionId=${event.sessionId}`);
      log(`response length=${response.length} first 200: ${truncate(response, 200)}`);

      fireAndForgetIngest(response, "model", event, ctx);
    });
  },
};
