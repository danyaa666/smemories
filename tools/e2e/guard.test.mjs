// Proves the refusals of guard.mjs: `cd tools/e2e && npm run guard-test` (make e2e and the e2e CI job run it).
import assert from "node:assert/strict";
import { test } from "node:test";
import { assertSafeTarget, dsnHost, problems } from "./guard.mjs";

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

test("a database host that is not loopback or a private address is refused, even with E2E_ALLOW_REMOTE=1", () => {
  for (const host of ["tcp(db.example.com:3306)", "tcp(8.8.8.8:3306)", "tcp(172.32.0.1:3306)", "tcp(127.0.0.1.evil.com:3306)", "tcp([2001:db8::1]:3306)", "tcp(11.0.0.1)", "tcp(broken"]) {
    const env = { SMEM_DB_DSN: `u:p@${host}/smem_test_e2e`, E2E_ALLOW_REMOTE: "1" };
    assert.equal(problems(env).length, 1, host);
    assert.throws(() => assertSafeTarget(env), /not loopback or a private address/);
  }
  // A password with brackets, slashes and an @ must not hide the host.
  assert.equal(problems({ SMEM_DB_DSN: "u:p(x)/@y@tcp(db.example.com:3306)/smem_test_e2e" }).length, 1);
});

test("loopback and private database hosts pass", () => {
  for (const host of ["tcp(127.0.0.1:3306)", "tcp(localhost:3306)", "tcp([::1]:3306)", "tcp(10.1.2.3:3306)", "tcp(172.16.0.9:3306)", "tcp(192.168.1.5:3306)", "tcp([fd12::1]:3306)", "unix(/tmp/mysql.sock)", ""]) {
    assert.deepEqual(problems({ SMEM_DB_DSN: `u:p@${host}/smem_test_e2e?parseTime=true` }), [], host);
  }
  assert.equal(dsnHost("u:p@tcp(10.0.0.1:3306)/smem_e2e"), "10.0.0.1");
});
