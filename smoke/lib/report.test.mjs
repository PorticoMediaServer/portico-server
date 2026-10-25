import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync, readFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createReport, assert as requireCheck} from './report.mjs';

// Checkpoint-only regression coverage; no browser, network or credentials.
test('a deployment-pin failure remains failed in the serialized checkpoint report', async () => {
  const out = mkdtempSync(join(tmpdir(), 'portico-smoke-report-'));
  try {
    const report = createReport({surface: 'hosted', origin: 'https://example.invalid', out});
    await report.check('boot', async () => ({detail: 'report fixture'}));
    report.blocked('optional-fixture', 'fixture unavailable');
    report.note('build-pin', 'deployment pin mismatch');
    await report.check('build-pin-valid', async () => {
      requireCheck(false, 'deployment does not match the requested build pin');
    });
    const result = report.write();
    const saved = JSON.parse(readFileSync(join(out, 'report.json'), 'utf8'));
    assert.equal(result.verdict, 'fail');
    assert.equal(saved.verdict, 'fail');
    assert.deepEqual(saved.counts, {pass: 1, fail: 1, blocked: 1});
    const pin = saved.checks.find(check => check.id === 'build-pin-valid');
    assert.equal(pin.status, 'fail');
    assert.equal(pin.required, true);
    assert.equal(pin.detail, 'deployment does not match the requested build pin');
  } finally {
    rmSync(out, {recursive: true, force: true});
  }
});
