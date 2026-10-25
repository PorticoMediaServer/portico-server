// Keep reading-session identities out of retained diagnostic reports.
export function readingResponsePath(pathname) {
  const match = /^\/v1\/reading\/sessions\/[^/]+\/(pages|source)(?:\/[^/]+)?\/?$/.exec(pathname);
  if (!match) return null;
  return `/v1/reading/sessions/:session/${match[1]}${match[1] === 'pages' ? '/:page' : ''}`;
}
