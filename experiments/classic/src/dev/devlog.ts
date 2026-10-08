/** Development only: records a line in the local command server's request log. */
export function devlog(msg: string) {
  if (!__DEV__) return;
  fetch(`http://localhost:8191/log/${encodeURIComponent(msg)}`).catch(() => {});
}


// Development only: copy warnings and errors to the same log, since the review
// tooling cannot see the debugger console.
if (__DEV__) {
  for (const level of ['warn', 'error'] as const) {
    const original = console[level];
    console[level] = (...args: unknown[]) => {
      devlog(`${level}: ${args.map(a => (a instanceof Error ? a.message : String(a))).join(' ').slice(0, 300)}`);
      original(...args);
    };
  }
}
