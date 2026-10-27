// Await each action before starting another; pausing never starts an extra call.
export async function advanceUntilDone(run, advance, isCurrent) {
  while (isCurrent() && run.auto && !run.done) {
    const before = run.index;
    await advance();
    if (run.fatal || run.index === before) break;
  }
}
