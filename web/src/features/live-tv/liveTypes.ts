import type {
  ActionableDVRRecording,
  ActionableDVRRule,
  ActionableLiveTVChannel,
  LiveTVGuideInput,
  LiveTVGuideResult,
  PorticoDataSource,
} from '../../data/models';

export type LiveTVGuidePageInput = LiveTVGuideInput & {
  limit: number;
  cursor?: string;
  group?: string;
  order?: 'asc' | 'desc';
};

export type LiveTVGuideWorkspacePage = LiveTVGuideResult & {
  channelGroups?: string[];
};

export type LiveTVChannelPageInput = {
  limit: number;
  cursor?: string;
  query?: string;
  favoritesOnly?: boolean;
  group?: string;
};

export type LiveTVChannelPage = {
  items: ActionableLiveTVChannel[];
  pageInfo: { nextCursor: string | null; hasMore: boolean; total?: number };
  groups?: string[];
};

export type DVRConflict = {
  id: string;
  recordingIds: string[];
  startsAt: string;
  endsAt: string;
  reason: string;
  capacity?: number;
  demand?: number;
  messageId?: string;
  actions: string[];
};

export type DVRConsumerStatus = {
  capabilities: {
    canScheduleRecordings: boolean;
    canManageRecordingRules: boolean;
    canCreateOwnRules: boolean;
    canEditOwnRules: boolean;
    canDeleteOwnRules: boolean;
    canManageAllRules: boolean;
    actions: string[];
  };
  conflicts: DVRConflict[];
  storage: {
    usedBytes: number;
    availableBytes: number;
    forecastDays?: number;
    state: 'healthy' | 'pressure' | 'full';
  };
  generatedAt: string;
};

export type DVRTunerAllocation = {
  id: string;
  name: string;
  state: 'idle' | 'live' | 'recording' | 'conflict' | 'offline';
  channelId?: string;
  recordingId?: string;
};

export type DVROperationalStatus = {
  capabilities: {
    canScheduleRecordings: boolean;
    canManageRecordingRules: boolean;
    actions: string[];
  };
  guide: {
    state: 'current' | 'stale' | 'missing' | 'source-offline';
    lastRefreshedAt?: string;
    message?: string;
  };
  conflicts: DVRConflict[];
  tuners: DVRTunerAllocation[];
  storage: {
    usedBytes: number;
    availableBytes: number;
    forecastDays?: number;
    state: 'healthy' | 'pressure' | 'full';
  };
};

export type DVRRecordingPageInput = {
  status?: Array<ActionableDVRRecording['status']>;
  query?: string;
  limit: number;
  cursor?: string;
};

export type DVRRulePageInput = {
  query?: string;
  limit: number;
  cursor?: string;
};

export type ResourcePage<T> = {
  items: T[];
  pageInfo: { nextCursor: string | null; hasMore: boolean; total?: number };
};

export type ExtendedLiveTVDataSource = PorticoDataSource & {
  liveTVGuidePage?: (sourceId: string, input: LiveTVGuidePageInput, signal: AbortSignal) => Promise<LiveTVGuideWorkspacePage>;
  liveTVChannelsPage?: (sourceId: string, input: LiveTVChannelPageInput, signal: AbortSignal) => Promise<LiveTVChannelPage>;
  dvrStatus?: (sourceId: string | undefined, signal: AbortSignal) => Promise<DVRConsumerStatus>;
  dvrRecordingsPage?: (input: DVRRecordingPageInput, signal: AbortSignal) => Promise<ResourcePage<ActionableDVRRecording>>;
  dvrRulesPage?: (input: DVRRulePageInput, signal: AbortSignal) => Promise<ResourcePage<ActionableDVRRule>>;
};

export type FeatureQueryState<T> =
  | { status: 'loading'; data?: undefined; error?: undefined }
  | { status: 'success'; data: T; error?: undefined }
  | { status: 'error'; data?: undefined; error: Error };
