/** Browser-only helper for the account data export (the feed and export client live in `@core/account-notifications.ts`). */
/** Saves a document as `portico-account.json` in the browser. */
export function saveJSON(data: unknown, name = 'portico-account.json'): void {
  const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], {type: 'application/json'}));
  const a = document.createElement('a');
  a.href = url; a.download = name; a.rel = 'noopener';
  document.body.append(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 30_000);
}
