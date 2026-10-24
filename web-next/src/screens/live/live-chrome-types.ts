import type {ChannelPlaybackFence} from '../../../../packages/player-core/src/index.ts';

/** The narrow facts the drawn Live chrome needs. Everything here is a genuine guide or
 * channel-session fact: a missing optional fact leaves its drawn element out, never guessed. */
export type LiveChromeChannel = Readonly<{
  id: string;
  sourceId: string;
  kind: 'live-source' | 'library-channel';
  generation: string;
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
  /** Absent when the session has no programme facts; never inferred from the media window. */
  programmeStartMs?: number;
  programmeEndMs?: number;
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

export type LiveChromeGuideTarget = Readonly<{sourceId: string; channelId: string; generation: string; kind: 'live-source' | 'library-channel'}>;

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
  /** The refreshed real DVR model owns the visible scheduled/recording/cancelled state. */
  record(): Promise<void>;
  goLive(): Promise<void>;
  tune(direction: -1 | 1): Promise<void>;
  tuneTo(target: LiveChromeGuideTarget): Promise<void>;
  openFullGuide(): void;
  openRecordings(): void;
}>;
