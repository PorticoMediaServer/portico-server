/**
 * What this browser draws itself, as told to the server: the one place the web player's
 * `subtitleRendering` list is decided and written into the capability document.
 *
 * The server burns a subtitle into the video only when the device has not said it can draw that
 * kind. The values are the protocol's: `text`, `ass`, `pgs`, `vobsub`, `dvb`.
 *
 * `declareSubtitleRendering` is called from this platform's `browserClientProfile`.
 * It writes `engines.native.subtitleRendering` when the
 * document has engines (the capability document), and the older `subtitles.styled` / `.bitmap`
 * lists when it does not.
 */
export type SubtitleRenderingKind = 'text' | 'ass' | 'pgs' | 'vobsub' | 'dvb';

/** Only the subtitle renderer actually mounted by this platform is declared. */
export function subtitleRenderingKinds(): SubtitleRenderingKind[] {
  // The Phase 1 platform runtime mounts the real text surface. Styled/image overlays belong to
  // Phase 4 and are not declared to the server until that actual renderer is integrated.
  return ['text'];
}

type Fields = Record<string, unknown>;
const record = (v: unknown): v is Fields => !!v && typeof v === 'object' && !Array.isArray(v);

/** The capability document with this browser's drawn subtitle kinds written in. */
export function declareSubtitleRendering<T>(document: T, kinds: readonly SubtitleRenderingKind[] = subtitleRenderingKinds()): T {
  if (!record(document)) return document;
  if (record(document.engines)) {
    const engines: Fields = {};
    for (const [name, engine] of Object.entries(document.engines)) engines[name] = record(engine) ? {...engine, subtitleRendering: [...kinds]} : engine;
    return {...document, engines} as T;
  }
  const before = record(document.subtitles) ? document.subtitles : {};
  return {...document, subtitles: {...before, styled: kinds.includes('ass') ? ['ass', 'ssa'] : [], bitmap: kinds.filter(kind => kind === 'pgs' || kind === 'vobsub' || kind === 'dvb')}} as T;
}
