import type {ChannelPlaybackFence} from '../../../../packages/player-core/src/index.ts';

/** The narrow facts the drawn Live chrome needs. Everything here is a genuine guide or
 * channel-session fact: a missing optional fact leaves its drawn element out, never guessed. */
export type LiveChromeChannel = Readonly<{
  id: string;
  sourceId: string;
  number: string;
  name: string;
  mark?: Readonly<{text: string; seed: string}>;
  favorite: boolean;
  recordAvailable: boolean;
  tuneAvailable: boolean;
}>;

export type LiveChromeProgramme = Readonly<{
  id: string;
  title: string;
  subtitle?: string;
  episode?: Readonly<{season?: number; number?: number; display?: string}>;
  startMs: number;
  endMs: number;
  /** The guide's own one-off recording fact for this airing. */
  recording?: 'recording' | 'scheduled';
}>;

export type LiveChromeWindow = Readonly<{
  programmeStartMs: number;
  programmeEndMs: number;
  /** The actual retained stream window, not the programme's wall-clock span. */
  seekStartMs: number;
  seekEndMs: number;
  liveEdgeMs: number;
  positionMs: number;
  behindSeconds: number;
  atLive: boolean;
}>;

export type LiveChromeGuideChannel = Readonly<{
  channel: LiveChromeChannel;
  programme?: LiveChromeProgramme;
  /** The wall-clock time the next airing starts (the current airing's end). */
  nextStartMs?: number;
}>;

export type LiveChromePort = Readonly<{
  /** The exact channel fence this chrome belongs to. */
  fence: ChannelPlaybackFence;
  channel: LiveChromeChannel;
  programme?: LiveChromeProgramme;
  nextProgramme?: Readonly<{title: string; startMs: number; endMs: number}>;
  window: LiveChromeWindow;
  /** Actual lineup neighbours; absent when the guide has no such row. */
  down?: LiveChromeGuideChannel;
  up?: LiveChromeGuideChannel;
  miniGuide: readonly LiveChromeGuideChannel[];
  skipBackSeconds: number;
  skipForwardSeconds: number;
  /** Rechecks the runtime's active fence, authority and this channel's source generation. */
  isCurrent(): boolean;
  /** Server-confirmed outcomes only. */
  record(): Promise<'recorded' | 'cancelled'>;
  goLive(): Promise<void>;
  tune(direction: -1 | 1): Promise<void>;
  tuneTo(channelId: string): Promise<void>;
  openFullGuide(): void;
  openRecordings(): void;
}>;
