// T-157: a foreign server on the check's port must make startServer fail fast; it must never be used or killed.
import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import http from "node:http";
import { test } from "node:test";
import { promisify } from "node:util";

test("a foreign server on the port fails the start and stays up", async () => {
  const foreign = http.createServer((_, res) => res.end("foreign")).listen(0, "localhost");
  await new Promise((r) => foreign.once("listening", r));
  const port = foreign.address().port;
  try {
    const run = promisify(execFile)(process.execPath, ["--input-type=module", "-e", 'import { startServer } from "./lib.mjs"; (await startServer())();'], { env: { ...process.env, PRINT_CHECK_PORT: String(port) } });
    await assert.rejects(run, (e) => e.stderr.includes(`port ${port}`) && e.stderr.includes("never tests a server it did not start"));
    assert.equal(await (await fetch(`http://localhost:${port}`)).text(), "foreign", "the foreign server was not touched");
  } finally {
    foreign.close();
  }
});
