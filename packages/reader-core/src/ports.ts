/** Binary access is supplied by the selected server/viewer scope, or a lawful
 * local/offline file. Engines receive no URL, cookie, token or HTTP client. */
export type ReaderRange = Readonly<{start: number; endExclusive: number}>;
export interface ReaderSource {
  readonly id: string;
  readonly revision: string;
  readonly size: number;
  readonly resources?: ReaderResources;
  /** Captured manifest entries for this source, never caller-supplied URLs. */
  readonly contents?: readonly ReaderContentsEntry[];
  /** Genuine interactive from-start stream; never a synthetic random reader. */
  readonly sequential?: ReaderSequentialSource;
  read?(range: ReaderRange, signal: AbortSignal): Promise<Uint8Array>;
}
export type ReaderSequentialSource = Readonly<{open(signal: AbortSignal): Promise<ReadableStream<Uint8Array>>}>;
export type ReaderResource = Readonly<{path: string; mime: string; size: number}>;
export interface ReaderResources {
  readonly entries: readonly ReaderResource[];
  read(path: string, signal: AbortSignal): Promise<Uint8Array>;
}
export type ReaderFormat = 'epub' | 'pdf' | 'cbz';
export type ReaderViewport = Readonly<{width: number; height: number}>;
export type ReaderRect = Readonly<{x: number; y: number; width: number; height: number}>;
type ObservedLocation = Readonly<{
  progression: number;
  /** These are omitted unless observed by the renderer; never estimated copy. */
  chapter?: string;
  pageLabel?: string;
  pagesLeftInSection?: number;
}>;
export type ReaderLocation = ObservedLocation & (
  | Readonly<{format: 'epub'; cfi: string; section?: string}>
  | Readonly<{format: 'pdf' | 'cbz'; page: number}>
);
export type ReaderTocItem = Readonly<{
  id: string; label: string; location: ReaderLocation; children?: readonly ReaderTocItem[];
}>;
export type ReaderContentsEntry = Readonly<{title: string; depth: number; section: string; fragment?: string}>;
/** A detail selection can cross a session reopen only for the same source revision. */
export type ReaderContentsTarget = Readonly<{sourceId: string; sourceRevision: string; index: number}>;
export type ReaderMetadata = Readonly<{
  title?: string; author?: string; language?: string;
  direction?: 'ltr' | 'rtl' | 'vertical'; pageCount?: number;
}>;
export type ReaderSelection = Readonly<{
  text: string; location: ReaderLocation; rects: readonly ReaderRect[];
}>;
export type ReaderSearchResult = Readonly<{
  id: string; excerpt: string; location: ReaderLocation; chapter?: string;
}>;
export type ReaderPageImage = Readonly<{mime: 'image/jpeg' | 'image/png' | 'image/webp'; bytes: Uint8Array; width: number; height: number}>;
export type ReaderSearchOptions = Readonly<{signal: AbortSignal; limit: number}>;
export type ReaderNavigation =
  | Readonly<{kind: 'next' | 'previous'}>
  | Readonly<{kind: 'location'; location: ReaderLocation}>
  | Readonly<{kind: 'progression'; value: number}>;
export type ReaderFit = 'page' | 'width' | 'height' | 'spread';
export type ReaderZoom = Readonly<{kind: 'scale'; value: number}> | Readonly<{kind: 'fit'; value: ReaderFit}>;
export type ReaderZoomState = Readonly<{scale: number; fit?: ReaderFit}>;
export type ReaderErrorCode = 'aborted' | 'closed' | 'invalid_source' | 'invalid_book'
  | 'invalid_location' | 'invalid_preferences' | 'limit_exceeded' | 'protected'
  | 'unsupported_format' | 'unsupported_operation' | 'source_unavailable' | 'render_failed';

/** No persistence API is exposed: Aa and fixed-page changes are session-only.
 * Preferences is the canonical Reading facade value, supplied by the host. */
export interface ReaderView<Preferences extends object> {
  navigate(target: ReaderNavigation, signal: AbortSignal): Promise<void>;
  applyPreferences(preferences: Readonly<Preferences>, signal: AbortSignal): Promise<void>;
  resize(viewport: ReaderViewport, signal: AbortSignal): Promise<void>;
  getLocation(): ReaderLocation | null;
  /** Transient presentation of a real search result; no bookmark/annotation persistence. */
  showSearchResult?(location: ReaderLocation | null, signal: AbortSignal): Promise<void>;
  setZoom?(zoom: ReaderZoom, signal: AbortSignal): Promise<void>;
  getZoom?(): ReaderZoomState;
  canTurnAtFit?(): boolean;
  pan?(offset: Readonly<{x: number; y: number}>, signal: AbortSignal): Promise<void>;
  dispose(): void | Promise<void>;
}
export type ReaderMount<Preferences extends object> = Readonly<{
  container: HTMLElement; preferences: Readonly<Preferences>; viewport: ReaderViewport;
  /** Resolved catalogue identity, including authoritative omitted fields. */
  metadata?: ReaderMetadata;
  signal: AbortSignal; onLocation(location: ReaderLocation): void;
  onSelection(selection: ReaderSelection | null): void;
  onSurfaceInput?(input: ReaderSurfaceInput): void;
}>;
export interface ReaderDocument<Preferences extends object> {
  readonly id: string;
  readonly format: ReaderFormat;
  readonly revision: string;
  readonly metadata: ReaderMetadata;
  readonly toc: readonly ReaderTocItem[];
  /** Resolve a captured manifest entry through this already-open engine. */
  resolveContents?(index: number, signal: AbortSignal): Promise<ReaderLocation>;
  mount(options: ReaderMount<Preferences>): Promise<ReaderView<Preferences>>;
  search(query: string, options: ReaderSearchOptions): AsyncIterable<ReaderSearchResult>;
  /** Real fixed-page preview; width 32..2048, bytes at most 16MiB. */
  page?(index: number, options: Readonly<{width: number; signal: AbortSignal}>): Promise<ReaderPageImage>;
  dispose(): void | Promise<void>;
}
export interface ReaderAdapter<Preferences extends object> {
  readonly format: ReaderFormat;
  open(options: Readonly<{source: ReaderSource; signal: AbortSignal}>): Promise<ReaderDocument<Preferences>>;
}
export type ReaderSurfaceInput =
  | Readonly<{kind: 'tap'; fraction: number}>
  | Readonly<{kind: 'activity'}>
  | Readonly<{kind: 'key'; key: string; shift: boolean; alt: boolean; ctrl: boolean; meta: boolean; editable: boolean; preventDefault(): void}>;
export type ReaderSnapshot<Preferences extends object> = Readonly<{
  status: 'closed' | 'opening' | 'ready' | 'error';
  zoom: ReaderZoomState | null; canTurnAtFit: boolean;
  format?: ReaderFormat; sourceId?: string; sourceRevision?: string; metadata?: ReaderMetadata;
  toc: readonly ReaderTocItem[]; location: ReaderLocation | null;
  selection: ReaderSelection | null; preferences: Readonly<Preferences>;
  overriddenKeys: readonly (keyof Preferences)[]; error?: ReaderErrorCode;
}>;
