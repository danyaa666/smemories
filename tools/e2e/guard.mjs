// Refuses to run the journey against anything but a throwaway local target (T-087, docs/e2e.md). Used by playwright.config.mjs and,
// as `node tools/e2e/guard.mjs`, by scripts/e2e.sh before it builds or starts anything. Pure checks, so guard.test.mjs can prove them.
import { fileURLToPath } from "node:url";

const LOCAL_HOSTS = new Set(["localhost", "127.0.0.1", "[::1]"]);

/** Database name of a go-sql-driver DSN (`user:pw@tcp(host:3306)/name?params`); "" when there is none. */
export const dbName = (dsn) => dsn.slice(dsn.lastIndexOf("/") + 1).split("?")[0];

/** The reasons to refuse (empty = go ahead). `env` is process.env. */
export function problems(env) {
  const out = [];
  const url = env.E2E_BASE_URL;
  if (url && env.E2E_ALLOW_REMOTE !== "1") {
    let host = "";
    try {
      host = new URL(url).hostname;
    } catch {
      /* unparsable: refused below */
    }
    if (!LOCAL_HOSTS.has(host)) {
      out.push(`E2E_BASE_URL host "${host || url}" is not localhost, 127.0.0.1 or ::1 (E2E_ALLOW_REMOTE=1 allows a remote target on purpose)`);
    }
  }
  const dsn = env.SMEM_DB_DSN;
  if (dsn !== undefined && dsn !== "" && !/^smem_(e2e|test)/.test(dbName(dsn))) {
    out.push(`SMEM_DB_DSN database "${dbName(dsn)}" does not start with smem_e2e or smem_test; the journey writes to it (make e2e creates smem_test_e2e)`);
  }
  return out;
}

export function assertSafeTarget(env = process.env) {
  const p = problems(env);
  if (p.length) throw new Error(`e2e: refusing to run:\n  - ${p.join("\n  - ")}`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    assertSafeTarget();
  } catch (e) {
    console.error(e.message);
    process.exit(2);
  }
}
