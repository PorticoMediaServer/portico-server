import type {ReadingManifest} from '@portico/client-core';
import type {ReaderContentsEntry} from '../../kit/reader/ReaderPanel';

type Contents = NonNullable<ReadingManifest['contents']>;
type Entry = {id: string; label: string; onNavigate: () => void; children?: Entry[]};

/** Keep declared publisher order and original target indices. Depth alone
 * supplies nesting; no chapter numbers, page labels or progress are inferred. */
export function readingContentsEntries(contents: Contents, navigate: (index: number) => void): readonly ReaderContentsEntry[] {
  const roots: Entry[] = [], parents: {depth: number; entry: Entry}[] = [];
  contents.forEach((item, index) => {
    const entry: Entry = {id: String(index), label: item.title, onNavigate: () => navigate(index)};
    while (parents.length && parents[parents.length - 1]!.depth >= item.depth) parents.pop();
    const parent = parents[parents.length - 1]?.entry;
    if (parent) (parent.children ??= []).push(entry); else roots.push(entry);
    parents.push({depth: item.depth, entry});
  });
  return roots;
}
