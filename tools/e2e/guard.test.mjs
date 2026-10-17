// Proves the refusals of guard.mjs: `cd tools/e2e && npm run guard-test` (make e2e and the e2e CI job run it).
import assert from "node:assert/strict";
import { test } from "node:test";
import { assertSafeTarget, problems } from "./guard.mjs";

const DSN = (db) => `u:p@w/d@tcp(127.0.0.1:3306)/${db}`;

test("local base URLs and e2e/test databases pass", () => {
  for (const url of ["http://127.0.0.1:14173", "http://localhost:3000", "http://[::1]:1/"]) assert.deepEqual(problems({ E2E_BASE_URL: url }), []);
  for (const db of ["smem_e2e", "smem_test_e2e", "smem_e2e_7?parseTime=true"]) assert.deepEqual(problems({ SMEM_DB_DSN: DSN(db) }), []);
  assert.deepEqual(problems({}), []);
});

test("a non-local base URL is refused, and E2E_ALLOW_REMOTE=1 overrides that check only", () => {
  const env = { E2E_BASE_URL: "https://staging.example.com" };
  assert.match(problems(env)[0], /staging\.example\.com/);
  assert.throws(() => assertSafeTarget(env), /refusing to run/);
  assert.deepEqual(problems({ ...env, E2E_ALLOW_REMOTE: "1" }), []);
  assert.equal(problems({ E2E_BASE_URL: "not a url" }).length, 1);
  assert.equal(problems({ E2E_BASE_URL: "http://127.0.0.1.evil.com" }).length, 1);
  assert.equal(problems({ E2E_BASE_URL: "http://localhost@evil.com" }).length, 1);
});

test("a database whose name is not smem_e2e* or smem_test* is refused, even with E2E_ALLOW_REMOTE=1", () => {
  for (const db of ["smemories", "", "prod_smem_e2e", "x?smem_test"]) {
    const env = { SMEM_DB_DSN: DSN(db), E2E_ALLOW_REMOTE: "1" };
    assert.equal(problems(env).length, 1, db);
    assert.throws(() => assertSafeTarget(env), /smem_e2e or smem_test/);
  }
});
