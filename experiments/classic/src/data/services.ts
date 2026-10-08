import type * as T from './types';

/**
 * The only way screens reach data. `FixtureClient` implements it on bundled
 * fixtures; a real implementation would wrap Portico's HTTP API (the shapes
 * mirror packages/client-core). Mutations resolve when the server confirms;
 * `subscribe` notifies screens that a topic changed so they refetch.
 */
export type Topic = 'personal' | 'home' | 'saved' | 'live' | 'dvr' | 'downloads' | 'together' | 'session' | 'notifications' | 'prefs' | 'admin' | 'libraries' | 'account';

export interface PorticoClient {
  subscribe(topic: Topic, fn: () => void): () => void;

  session: {
    current(): T.Session | undefined;
    servers(): Promise<T.Server[]>;
    profiles(serverId?: string): Promise<T.Profile[]>;
    signInAccount(identity: string, password: string): Promise<{twoStep: boolean}>;
    verifyCode(code: string): Promise<void>;
    signInDirect(host: string, username: string, password: string): Promise<{twoStep: boolean; mustChangePassword: boolean}>;
    chooseServer(serverId: string): Promise<{status: T.ServerStatus}>;
    chooseProfile(profileId: string, pin?: string): Promise<void>;
    signOut(): Promise<void>;
    createAccount(input: {username: string; email: string; password: string}): Promise<void>;
    requestPasswordReset(identity: string): Promise<void>;
    resetPassword(password: string, code?: string): Promise<void>;
    verifyEmail(token: string): Promise<'confirmed' | 'expired'>;
    activationCode(): Promise<{code: string; url: string; expiresIn: number}>;
    pollActivation(code: string): Promise<'waiting' | 'approved' | 'declined'>;
    approveDevice(code: string): Promise<{device: string; platform: string; location: string}>;
    confirmDevice(code: string, approve: boolean): Promise<void>;
    invitation(code: string): Promise<{server: string; inviter: string; libraries: number; state: 'valid' | 'expired' | 'used'}>;
    acceptInvitation(code: string, how: {account?: boolean; username?: string; password?: string}): Promise<void>;
    declineInvitation(code: string): Promise<void>;
    claimRequest(): Promise<{server: string; address: string; requested: string; state: 'waiting' | 'approved' | 'denied'}>;
    answerClaim(approve: boolean): Promise<void>;
    setupServer(input: {name: string; mode: 'hosted' | 'local'; owner: string; password: string}): Promise<void>;
    restore(): Promise<'restored' | 'unreachable'>;
    connectivity(): 'online' | 'reconnecting' | 'offline';
    setConnectivity(state: 'online' | 'reconnecting' | 'offline'): void;
  };

  libraries: {
    list(): Promise<T.Library[]>;
    discover(libraryId: string): Promise<T.HomeRow[]>;
    browse(libraryId: string, query: T.BrowseQuery, offset: number, limit: number, view?: T.LibraryView): Promise<T.BrowsePage>;
    letters(libraryId: string, query: T.BrowseQuery, view?: T.LibraryView): Promise<T.LetterIndex>;
    facets(libraryId: string): Promise<T.Facet[]>;
    collections(libraryId: string): Promise<T.Entry[]>;
    categories(libraryId: string): Promise<T.Category[]>;
    entity(libraryId: string, kind: string, id: string): Promise<{title: string; subtitle?: string; image?: T.Img; backdrop?: T.Img; overview?: string; entries: T.Entry[]; shape: T.Shape}>;
    scan(libraryId: string): Promise<void>;
  };

  home: {
    document(): Promise<T.HomeDocument>;
    row(rowId: string): Promise<T.HomeRow>;
    layout(): Promise<T.HomeLayoutRow[]>;
    saveLayout(rows: T.HomeLayoutRow[]): Promise<void>;
    resetLayout(): Promise<void>;
    removeFromContinue(itemId: string): Promise<void>;
    restoreToContinue(itemId: string): Promise<void>;
  };

  catalog: {
    movie(id: string): Promise<T.MovieDetail>;
    show(id: string): Promise<T.ShowDetail>;
    episode(id: string): Promise<T.EpisodeDetail>;
    person(id: string): Promise<T.PersonDetail>;
    artist(id: string): Promise<T.ArtistDetail>;
    album(id: string): Promise<T.AlbumDetail>;
    audiobook(id: string): Promise<T.AudiobookDetail>;
    photoAlbum(id: string): Promise<T.PhotoAlbumDetail>;
    homeVideo(id: string): Promise<T.MovieDetail>;
    entry(id: string): T.Entry | undefined;
    tracks(ids: string[]): T.TrackItem[];
    matchCandidates(id: string, query?: string): Promise<{id: string; title: string; year: number; provider: string; score: number; image?: T.Img}[]>;
    metadata(id: string): Promise<Record<string, string>>;
    saveMetadata(ids: string[], fields: Record<string, string>, lock: boolean): Promise<void>;
    fixMatch(id: string, candidateId: string): Promise<void>;
    refreshMetadata(ids: string[]): Promise<void>;
    deleteMedia(ids: string[], removeFiles: boolean): Promise<void>;
  };

  personal: {
    get(id: string): T.Personal;
    setWatched(ids: string[], watched: boolean): Promise<void>;
    setWatchlist(ids: string[], on: boolean): Promise<void>;
    setFavorite(ids: string[], on: boolean): Promise<void>;
    rate(id: string, stars: number | undefined): Promise<void>;
    react(id: string, reaction: 'like' | 'dislike' | undefined): Promise<void>;
    notInterested(id: string, on: boolean): Promise<void>;
    setProgress(id: string, seconds: number): void;
  };

  search: {
    query(q: string, group?: T.SearchGroupId): Promise<T.SearchResults>;
    recent(): string[];
    remember(q: string): void;
    clearRecent(): void;
  };

  saved: {
    list(tab: T.SavedTab, opts?: {filter?: 'all' | 'unwatched' | 'inProgress'; sort?: string; period?: string}): Promise<{entries: T.Entry[]; resources: T.SavedResource[]; history: T.HistoryEntry[]}>;
    resource(kind: string, id: string): Promise<T.PlaylistDetail>;
    createPlaylist(name: string, itemIds?: string[]): Promise<string>;
    createCollection(name: string, itemIds?: string[]): Promise<string>;
    addToPlaylist(playlistId: string, itemIds: string[]): Promise<void>;
    addToCollection(collectionId: string, itemIds: string[]): Promise<void>;
    removeFromResource(resourceId: string, itemId: string): Promise<void>;
    moveInResource(resourceId: string, itemId: string, delta: number): Promise<void>;
    renameResource(resourceId: string, name: string): Promise<void>;
    deleteResource(resourceId: string): Promise<void>;
    setShared(resourceId: string, shared: boolean): Promise<void>;
    saveView(libraryId: string, name: string, query: T.BrowseQuery): Promise<string>;
    clearHistory(): Promise<void>;
    playlists(): Promise<T.SavedResource[]>;
    collections(): Promise<T.SavedResource[]>;
  };

  live: {
    sources(): Promise<T.ChannelSource[]>;
    channels(opts?: {source?: string; group?: string; favorites?: boolean; hidden?: boolean}): Promise<T.Channel[]>;
    guide(from: number, to: number, channelIds?: string[]): Promise<Record<string, T.Program[]>>;
    program(id: string): Promise<{program: T.Program; channel: T.Channel; next?: T.Program}>;
    nowPlaying(channelId: string): T.Program | undefined;
    /** Asks the server to start a channel; says why when it can't. */
    start(channelId: string): Promise<{ok: true} | {ok: false; reason: 'tunersBusy' | 'sourceUnavailable' | 'storage' | 'stalled' | 'startFailed'}>;
    setFavorite(channelId: string, on: boolean): Promise<void>;
    setHidden(channelId: string, on: boolean): Promise<void>;
    remind(programId: string, on: boolean): Promise<void>;
    reminders(): string[];
    record(programId: string): Promise<void>;
    recordSeries(programId: string, opts: {channel: 'this' | 'any'; newOnly: boolean}): Promise<void>;
    cancelRecording(recordingId: string): Promise<void>;
    recordings(): Promise<{upcoming: T.Recording[]; recorded: T.Recording[]; conflicts: T.Recording[]; rules: T.SeriesRule[]; storage: T.DvrStorage; history: T.Recording[]}>;
    updateRule(id: string, patch: Partial<T.SeriesRule>): Promise<void>;
    deleteRule(id: string, cancelUpcoming: boolean): Promise<void>;
    deleteRecording(id: string): Promise<void>;
    keepRecording(id: string, keep: boolean): Promise<void>;
    resolveConflict(recordingId: string, choice: 'record' | 'skip'): Promise<void>;
  };

  together: {
    groups(): Promise<T.WatchGroup[]>;
    current(): T.WatchGroup | undefined;
    create(name: string, control: 'host-only' | 'anyone'): Promise<T.WatchGroup>;
    join(code: string): Promise<T.WatchGroup>;
    leave(): Promise<void>;
    end(): Promise<void>;
    watch(itemId: string): Promise<void>;
    queueAdd(itemId: string): Promise<void>;
    queueRemove(itemId: string): Promise<void>;
    makeHost(memberId: string): Promise<void>;
    newCode(): Promise<string>;
    setPlaying(playing: boolean): Promise<void>;
  };

  downloads: {
    list(): Promise<{items: T.Download[]; requests: T.DownloadRequest[]; usedGB: number; freeGB: number; serverSpaceGB: number}>;
    add(itemId: string, quality: string): Promise<void>;
    addShow(showId: string, policy: 'all' | 'unwatched' | 'next', keep: number): Promise<void>;
    remove(id: string): Promise<void>;
    pause(id: string, paused: boolean): Promise<void>;
    retry(id: string): Promise<void>;
    settings(): T.DownloadSettings;
    updateSettings(patch: Partial<T.DownloadSettings>): Promise<void>;
    has(itemId: string): boolean;
  };

  account: {
    devices(): Promise<T.Device[]>;
    signOutDevice(id: string): Promise<void>;
    signOutEverywhere(): Promise<void>;
    trustDevice(id: string, trusted: boolean): Promise<void>;
    renameDevice(id: string, name: string): Promise<void>;
    changePassword(current: string, next: string): Promise<void>;
    setTwoStep(on: boolean, code?: string): Promise<{recoveryCodes?: string[]}>;
    recoveryCodes(): Promise<string[]>;
    updateProfile(id: string, patch: Partial<T.Profile> & {pinValue?: string | null}): Promise<void>;
    addProfile(name: string, art: string, kids: boolean): Promise<T.Profile>;
    deleteProfile(id: string): Promise<void>;
    moveProfile(id: string, delta: number): Promise<void>;
    invitations(): Promise<T.Invitation[]>;
    answerInvitation(id: string, accept: boolean): Promise<void>;
    leaveServer(serverId: string): Promise<void>;
    changeEmail(email: string): Promise<void>;
    exportData(): Promise<void>;
    deleteAccount(confirm: string): Promise<void>;
    connectProvider(p: 'google' | 'apple', on: boolean): Promise<void>;
  };

  notifications: {
    list(view: 'unread' | 'all' | 'archived'): Promise<T.Notice[]>;
    unread(): number;
    mark(id: string, read: boolean): Promise<void>;
    archive(id: string, archived: boolean): Promise<void>;
    markAll(): Promise<void>;
    sendFeedback(input: {kind: string; category: string; message: string; itemId?: string; diagnostics: boolean}): Promise<'sent' | 'duplicate'>;
  };

  prefs: {
    defs(): T.PrefDef[];
    get(key: string): T.PrefValue;
    set(key: string, value: T.PrefValue): Promise<void>;
    resetRecommendations(): Promise<void>;
  };
}
