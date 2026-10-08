import type {Server} from '../data/types';

/** One line about a server: whose it is, then how many libraries or when it was last seen. Its status goes in a badge. */
export function serverLine(s: Server, opts: {host?: boolean} = {}): string {
  const who = s.yours ? 'Yours' : `Shared by ${s.owner}`;
  if (s.status !== 'online') return `${who} · Last seen ${s.lastSeen}`;
  return `${who} · ${opts.host ? s.host : `${s.libraries} ${s.libraries === 1 ? 'library' : 'libraries'}`}`;
}
