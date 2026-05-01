import { appendFileSync, mkdirSync } from "fs";
import { homedir } from "os";
import { dirname } from "path";

const SEMCOM_DIR = process.env.SEMCOM_DIR ?? `${homedir()}/.local/share/semcom`;
const SEMCOM_URL = process.env.SEMCOM_URL ?? "http://localhost:8080";
const TIMEOUT_MS = 3000;
const TOP_K = 5;
const LOG_FILE = `${SEMCOM_DIR}/semcom-hooks.log`;

function log(msg) {
  const line = `[${new Date().toISOString()}] [semcom-plugin] ${msg}\n`;
  try {
    mkdirSync(dirname(LOG_FILE), { recursive: true });
    appendFileSync(LOG_FILE, line);
  } catch {}
}

async function semcomPost(body) {
  return fetch(`${SEMCOM_URL}/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(TIMEOUT_MS),
  });
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
  description: "Injects relevant semantic memories from semcom before each reply; ingests agent replies after delivery.",

  register(api) {
    log("plugin registered");

    api.on("before_prompt_build", async (event, ctx) => {
      const prompt = event.prompt;
      if (!prompt) return;
      log(`before_prompt_build — prompt length=${prompt.length}`);
      try {
        const resp = await semcomPost({
          operation: "chat",
          prompt,
          by: "user",
          top_k: TOP_K,
        });
        if (!resp.ok) {
          log(`semcom returned HTTP ${resp.status}`);
          return;
        }
        const data = await resp.json();
        const hits = data.context ?? [];
        if (hits.length === 0) {
          log("semcom returned 0 context hits");
          return;
        }
        log(`semcom returned ${hits.length} hit(s): ${hits.map(h => `id=${h.id} score=${h.score}`).join(", ")}`);
        return { prependContext: formatMemoryBlock(hits) + "\n\n" };
      } catch (err) {
        log(`semcom unreachable or error: ${err?.message ?? err}`);
      }
    });

    api.on("message_sent", (event, ctx) => {
      const content = event.content;
      if (!content) return;
      log(`message_sent — ingesting model reply (length=${content.length})`);
      semcomPost({ operation: "ingest", prompt: content, by: "model" }).catch(
        (err) => log(`ingest error: ${err?.message ?? err}`)
      );
    });
  },
};
