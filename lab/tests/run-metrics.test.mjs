import test from 'node:test';
import assert from 'node:assert/strict';
import { createMetrics, finishRequest, summarizeMetrics } from '../internal/app/web/run-metrics.js';

test('processing time excludes reading pauses and includes failed requests', () => {
  const metrics = createMetrics();
  finishRequest(metrics, 'Jev', 100, 160, { navigationMs: 50 });
  // The user waits ten seconds before requesting the next action.
  finishRequest(metrics, 'OpenAI', 10160, 11160, { generationMs: 900, toolMs: 20 });
  finishRequest(metrics, 'Failed action', 20000, 20200, { generationMs: 190 }, true);
  const total = summarizeMetrics(metrics, 60000);
  assert.equal(total.requestMs, 1260);
  assert.equal(total.navigationMs, 50);
  assert.equal(total.generationMs, 1090);
  assert.equal(metrics.requests[2].failed, true);
  metrics.active = { start: 60000 };
  assert.equal(summarizeMetrics(metrics, 60150).requestMs, 1410);
  assert.equal(summarizeMetrics(metrics, 70000).generationMs, 1090);
});
