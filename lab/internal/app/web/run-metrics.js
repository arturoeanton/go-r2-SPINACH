// Count request intervals, never the human pauses between Next step clicks.
export function createMetrics() {
  return { requests: [], active: null };
}

const positive = value => Number.isFinite(value) && value >= 0 ? value : 0;

export function finishRequest(metrics, label, start, end, timing, failed = false) {
  metrics.requests.push({ label, durationMs: positive(end - start), timing, failed });
  metrics.active = null;
}

export function summarizeMetrics(metrics, now) {
  const result = { requestMs: 0, navigationMs: 0, generationMs: 0, replayMs: 0, toolMs: 0 };
  for (const request of metrics.requests) {
    result.requestMs += positive(request.durationMs);
    for (const key of ['navigationMs', 'generationMs', 'replayMs', 'toolMs']) {
      result[key] += positive(request.timing?.[key]);
    }
  }
  if (metrics.active) result.requestMs += positive(now - metrics.active.start);
  return result;
}

export function formatDuration(ms) {
  return ms < 1000 ? `${ms.toFixed(1)} ms` : `${(ms / 1000).toFixed(2)} s`;
}
