// Tests of scrub-trace.sh (T-115): it redacts the secrets, repacks without directory entries (Playwright's reader fails on them) and
// fails closed (a trace it cannot scrub is deleted, the script still exits 0). Run: npm run guard-test
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const script = join(dirname(fileURLToPath(import.meta.url)), 'scrub-trace.sh');
const TOKEN = 'AbCdEf0123456789AbCdEf0123456789';

// A results folder with results/run1/trace.zip holding a nested file, made with directory entries like Playwright's own zip.
function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'scrub-'));
  const src = join(root, 'src');
  mkdirSync(join(src, 'resources'), { recursive: true });
  writeFileSync(join(src, 'trace.network'), `{"url":"http://x/api/photos/get-content?token=${TOKEN}","h":"/c/${TOKEN}"}\n`);
  writeFileSync(join(src, 'resources', 'r.txt'), `{"token":"${TOKEN}"} __Host-smem_session=${TOKEN};\n`);
  const results = join(root, 'results', 'run1');
  mkdirSync(results, { recursive: true });
  execFileSync('zip', ['-qr', join(results, 'trace.zip'), '.'], { cwd: src }); // with directory entries
  return { root, zip: join(results, 'trace.zip'), results: join(root, 'results') };
}

test('redacts the secrets and repacks without directory entries', () => {
  const f = fixture();
  try {
    assert.match(execFileSync('unzip', ['-Z1', f.zip]).toString(), /\/\n/); // the fixture does have directory entries
    execFileSync(script, [f.results]);
    const entries = execFileSync('unzip', ['-Z1', f.zip]).toString().trim().split('\n');
    assert.deepEqual(entries.filter((e) => e.endsWith('/')), []);
    assert.ok(entries.includes('resources/r.txt'));
    const all = entries.map((e) => execFileSync('unzip', ['-p', f.zip, e]).toString()).join('');
    assert.ok(!all.includes(TOKEN), 'a token survived');
    assert.match(all, /token=REDACTED/);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test('fails closed: a trace that cannot be scrubbed is deleted, the exit code stays 0', () => {
  const f = fixture();
  try {
    writeFileSync(f.zip, 'this is not a zip');
    const r = spawnSync(script, [f.results], { encoding: 'utf8' });
    assert.equal(r.status, 0);
    assert.ok(!existsSync(f.zip), 'the unscrubbed trace is still there');
    assert.match(r.stderr, /DELETED .*could not be scrubbed/);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test('a missing results folder is fine', () => {
  assert.equal(spawnSync(script, [join(tmpdir(), 'no-such-results-dir')]).status, 0);
});
