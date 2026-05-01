import { spawn } from "child_process";
import { appendFileSync, existsSync, readFileSync } from "fs";
import { homedir } from "os";

const SEMCOM_DIR =
  process.env.SEMCOM_DIR ?? `${homedir()}/.local/share/semcom`;
const SEMCOM_PORT = process.env.SEMCOM_PORT ?? "8080";
const SEMCOM_URL = `http://localhost:${SEMCOM_PORT}`;
const LOG_FILE = `${SEMCOM_DIR}/semcom-hooks.log`;

function log(msg: string): void {
  const line = `[${new Date().toISOString()}] [semcom-start] ${msg}\n`;
  try { appendFileSync(LOG_FILE, line); } catch {}
}

function loadDotenv(dir: string): Record<string, string> {
  const path = `${dir}/.env`;
  if (!existsSync(path)) return {};
  const result: Record<string, string> = {};
  for (const line of readFileSync(path, "utf8").split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const eq = trimmed.indexOf("=");
    if (eq < 0) continue;
    const key = trimmed.slice(0, eq).trim();
    const val = trimmed.slice(eq + 1).trim().replace(/^["']|["']$/g, "");
    if (key) result[key] = val;
  }
  return result;
}

async function isRunning(): Promise<boolean> {
  try {
    const resp = await fetch(`${SEMCOM_URL}/chat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ operation: "ingest", prompt: "ping", by: "user" }),
      signal: AbortSignal.timeout(1500),
    });
    return resp.ok;
  } catch {
    return false;
  }
}

const handler = async (event: any): Promise<void> => {
  if (event.type !== "gateway" || event.action !== "startup") return;

  log("gateway:startup fired");

  const binary = `${SEMCOM_DIR}/semcom`;
  if (!existsSync(binary)) {
    log(`binary not found at ${binary} — skipping`);
    return;
  }

  if (await isRunning()) {
    log("semcom already running — nothing to do");
    return;
  }

  log("semcom not running — spawning...");
  const dotenv = loadDotenv(SEMCOM_DIR);

  const proc = spawn(binary, ["serve"], {
    env: {
      ...process.env,
      ...dotenv,
      PORT: dotenv.PORT ?? SEMCOM_PORT,
    },
    detached: true,
    stdio: "ignore",
  });
  proc.unref();
  log(`spawned semcom (pid ${proc.pid})`);
};

export default handler;
