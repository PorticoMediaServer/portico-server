import type {LibraryChannel} from '@core/library-channels.ts';

/** The shared action owner currently exposes one adjacent move. Reuse its
 * revisioned operation/receipt handling, and stop if any reread changes the
 * expected order. A lost receipt is left for that owner's explicit retry. */
export async function moveLibraryChannelTo(channel: LibraryChannel, target: number, ports: {
  current: () => boolean;
  rows: () => readonly LibraryChannel[] | undefined;
  move: (channel: LibraryChannel, direction: -1 | 1) => Promise<boolean>;
}): Promise<boolean> {
  const initial = ports.rows();
  if (!ports.current() || !initial?.includes(channel) || !Number.isInteger(target) || target < 0 || target >= initial.length) return false;
  let index = initial.indexOf(channel);
  const expected = initial.map(row => row.config.id), id = channel.config.id;
  if (new Set(expected).size !== expected.length) return false;
  for (let step = 0; index !== target && step < initial.length - 1; step++) {
    const rows = ports.rows();
    if (!ports.current() || !rows || rows.length !== expected.length || rows.some((row, position) => row.config.id !== expected[position])) return false;
    const selected = rows[index];
    if (!selected || selected.config.id !== id) return false;
    const direction = index < target ? 1 : -1, next = index + direction;
    if (!await ports.move(selected, direction) || !ports.current()) return false;
    [expected[index], expected[next]] = [expected[next]!, expected[index]!];
    index = next;
  }
  const final = ports.rows();
  return ports.current() && index === target && !!final && final.length === expected.length && final.every((row, position) => row.config.id === expected[position]);
}
