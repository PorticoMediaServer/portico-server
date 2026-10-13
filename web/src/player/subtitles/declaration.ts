/**
 * What this browser draws itself, as told to the server: the one place the web player's
 * `subtitleRendering` list is decided and written into the capability document.
 *
 * The server burns a subtitle into the video only when the device has not said it can draw that
 * kind. The values are the protocol's: `text`, `ass`, `pgs`, `vobsub`, `dvb`.
 *
 * To re-point: `declareSubtitleRendering` is called from `browserClientProfile` in
 * `web/src/bridge/client-profile.ts`. It writes `engines.native.subtitleRendering` when the
 * document has engines (the capability document), and the older `subtitles.styled` / `.bitmap`
 * lists when it does not.
 */
export type SubtitleRenderingKind = 'text' | 'ass' | 'pgs' | 'vobsub' | 'dvb';

export type RenderingFacts = {
  /** A canvas can be put over the video and workers exist. */
  canvas: boolean;
  /** WebAssembly can be compiled (styled subtitles need it). */
  webAssembly: boolean;
  /** The page can take the player full screen itself. Where only the video element can (iPhone),
   * full screen is the system's player, which shows the video and nothing drawn over it. */
  pageFullscreen: boolean;
};

function browserFacts(): RenderingFacts {
  const safe = (probe: () => boolean) => { try { return probe(); } catch { return false; } };
  return {
    canvas: safe(() => typeof document !== 'undefined' && typeof document.createElement('canvas').getContext === 'function' && typeof Worker === 'function'),
    webAssembly: safe(() => typeof WebAssembly === 'object' && typeof WebAssembly.instantiate === 'function'),
    pageFullscreen: safe(() => { const d = document as Document & {webkitFullscreenEnabled?: boolean}; return d.fullscreenEnabled === true || d.webkitFullscreenEnabled === true; }),
  };
}

/** The subtitle kinds this browser draws. Text always; the rest only where a drawn subtitle would be seen in every way the player shows video. */
export function subtitleRenderingKinds(facts: RenderingFacts = browserFacts()): SubtitleRenderingKind[] {
  const kinds: SubtitleRenderingKind[] = ['text'];
  if (!facts.canvas || !facts.pageFullscreen) return kinds;
  if (facts.webAssembly) kinds.push('ass');
  kinds.push('pgs', 'vobsub', 'dvb');
  return kinds;
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
