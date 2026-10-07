// Fails when src/api/schema.d.ts is stale relative to ../api/openapi.yaml.
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const dir = mkdtempSync(join(tmpdir(), "check-api-"));
try {
  const fresh = join(dir, "schema.d.ts");
  execFileSync("npx", ["--no-install", "openapi-typescript", "../api/openapi.yaml", "-o", fresh], {
    stdio: "ignore",
  });
  if (readFileSync(fresh, "utf8") !== readFileSync("src/api/schema.d.ts", "utf8")) {
    console.error("src/api/schema.d.ts is stale: run `npm run gen:api` and commit the result");
    process.exit(1);
  }
  console.log("api schema ok");
} finally {
  rmSync(dir, { recursive: true, force: true });
}
