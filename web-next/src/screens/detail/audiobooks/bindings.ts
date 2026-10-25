import {LibraryContentService, type ContentEntry, type ContentProjection} from '@core/library-content.ts';
import {getContainerState, setContainerState} from '@core/container-state.ts';
import {screenLifetime, type ScreenContext} from '../context';

export type AudiobookTarget = Readonly<{libraryId: string; view: 'book' | 'author' | 'book_series'; entityId: string}>;
export function createAudiobookBindings(context: ScreenContext, input: AudiobookTarget) {
  const target = Object.freeze({...input});
  const life = screenLifetime(context);
  const service = new LibraryContentService({api: life.api, scope: context.scope, pageSize: 40});
  const current = (page: ContentProjection) => {
    life.assert();
    const snapshot = service.getSnapshot();
    if (snapshot.phase !== 'ready' || snapshot.projection !== page || page.scope.libraryId !== target.libraryId
      || page.scope.libraryKind !== 'audiobook' || page.scope.view !== target.view || page.scope.entityId !== target.entityId
      || page.entity?.id !== target.entityId || page.entity.kind !== target.view) throw new DOMException('Audiobook page changed', 'AbortError');
  };
  const entries = () => service.getSnapshot().appended.flatMap(section => section.entries);
  const play = async (page: ContentProjection, entry: ContentEntry, startSeconds = entry.playback?.startSeconds ?? 0) => {
    current(page);
    if (!entries().includes(entry) || !entry.playback || entry.available === false || entry.missing) throw new DOMException('Audiobook target unavailable', 'AbortError');
    await context.effects.play({libraryId: target.libraryId, itemId: entry.playback.itemId, startSeconds}, life.signal(),
      () => {try {current(page);return entries().includes(entry);} catch {return false;}});
    current(page);
  };
  return {
    context, service, current,
    async load() {await service.select(target); const page = service.getSnapshot().projection; if (page) current(page);},
    async finished(page: ContentProjection) {current(page);if (target.view !== 'book') throw new DOMException('Not a book', 'AbortError');const state = await getContainerState(life.api, 'book', target.entityId, life.signal());current(page);return state;},
    async markFinished(page: ContentProjection) {
      current(page);
      if (target.view !== 'book') throw new DOMException('Not a book', 'AbortError');
      const state = await getContainerState(life.api, 'book', target.entityId, life.signal());current(page);
      const result = await setContainerState(life.api, 'book', target.entityId, {expectedRevision: state.revision, watched: !state.watched}, life.signal());
      current(page);return result;
    },
    play,
    async primary(page: ContentProjection, restart = false) {
      current(page);
      const rows = entries();
      if (target.view === 'book') {
        const journey = page.listening;
        if (!journey?.actions.includes('play') || journey.resumeUnavailable && !restart) throw new DOMException('Audiobook unavailable', 'AbortError');
        const resume = !restart ? journey.resume : undefined;
        if (resume) {
          // The server's resume may refer to a part outside this bounded window.
          await context.effects.play({libraryId: target.libraryId, itemId: resume.itemId, startSeconds: resume.positionSeconds}, life.signal(),
            () => {try {current(page);return true;} catch {return false;}});
          return;
        }
        const first = rows.find(entry => entry.kind === 'audiobook_file' && entry.playback) ?? rows.find(entry => entry.kind === 'chapter' && entry.playback);
        if (!first) throw new DOMException('Audiobook unavailable', 'AbortError');
        return play(page, first, 0);
      }
      const book = rows.find(entry => entry.kind === 'book' && entry.bookProgress && entry.available !== false && !entry.missing);
      if (!book?.bookProgress) throw new DOMException('Audiobook resume unavailable', 'AbortError');
      await context.effects.play({libraryId: target.libraryId, itemId: book.bookProgress.itemId, startSeconds: book.bookProgress.partPositionMs / 1000}, life.signal(),
        () => {try {current(page);return entries().includes(book);} catch {return false;}});
    },
    open(page: ContentProjection, entry: ContentEntry) {current(page);if (!entries().includes(entry)) throw new DOMException('Audiobook entry changed', 'AbortError');context.effects.openEntry(entry);},
    dispose() {life.dispose();service.dispose();},
  };
}
export type AudiobookBindings = ReturnType<typeof createAudiobookBindings>;
