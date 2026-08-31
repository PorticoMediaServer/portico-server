import {
  ApiError,
  type PlaybackResponse,
  type PlaybackSessionQueueResponse,
} from "@porticomediaserver/client-core";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router-dom";
import type { ReactNode } from "react";
import { DataProvider } from "../../data/DataProvider";
import { FixturePorticoDataSource } from "../../data/fixtureSource";
import type { PorticoDataSource, Viewer } from "../../data/models";
import {
  isExplicitMissingHLSManifest,
  isTransientHLSManifestWait,
  playableSubtitleStreams,
  PlaybackSessionProvider,
  PlayerDock,
  shouldUseNativeHLS,
  immutablePlaybackHandoffIntent,
  isAmbiguousPlaybackMutationFailure,
  usePlaybackSession,
  WatchPage,
} from "./PlayerSurface";
import { WebDisplayPreferencesProvider } from "../../preferences/WebDisplayPreferencesProvider";

const hlsFixtures = vi.hoisted(() => ({
  instances: [] as Array<{
    destroy: ReturnType<typeof vi.fn>;
    loadSource: ReturnType<typeof vi.fn>;
    config: { xhrSetup?: (request: XMLHttpRequest) => void };
  }>,
}));

vi.mock("hls.js", () => {
  class FixtureHls {
    static isSupported = () => true;
    static Events = {
      SUBTITLE_TRACKS_UPDATED: "subtitleTracksUpdated",
      MANIFEST_PARSED: "manifestParsed",
      FRAG_LOADED: "fragLoaded",
      ERROR: "error",
    };
    static ErrorTypes = {
      NETWORK_ERROR: "networkError",
      MEDIA_ERROR: "mediaError",
    };
    subtitleDisplay = false;
    subtitleTrack = -1;
    subtitleTracks: Array<{ lang?: string; name?: string }> = [];
    destroy = vi.fn();
    loadSource = vi.fn();
    attachMedia = vi.fn((media: HTMLMediaElement) =>
      queueMicrotask(() => media.dispatchEvent(new Event("loadedmetadata"))),
    );
    on = vi.fn();
    startLoad = vi.fn();
    recoverMediaError = vi.fn();
    constructor(public readonly config: { xhrSetup?: (request: XMLHttpRequest) => void }) {
      hlsFixtures.instances.push(this);
    }
  }
  return { default: FixtureHls };
});

const viewer: Viewer = {
  authenticated: true,
  setupRequired: false,
  serverName: "Portico Test",
  user: {
    id: "viewer",
    displayName: "Viewer",
    email: "viewer@example.test",
    role: "owner",
  },
};

function coreMedia(id: string, title: string): PlaybackResponse["media"] {
  return {
    id,
    entityKind: "episode",
    title,
    sortTitle: title,
    metadataEtag: `test-media-${id}-revision-1`,
    metadataRevision: 1,
    addedAt: "2026-01-01T00:00:00Z",
    durationSeconds: 120,
    genres: [],
    tags: [],
    labels: [],
    images: {
      poster: "/poster.jpg",
      backdrop: "/backdrop.jpg",
      thumb: "/thumb.jpg",
    },
    state: {
      watchlisted: false,
      favorite: false,
      watched: false,
      progressSeconds: 12,
      rating: 0,
    },
    actions: ["play" as const],
  };
}

function queueEntry(id: string, title: string, entryId = `entry-${id}`): PlaybackResponse["queue"][number] {
  return { entryId, media: coreMedia(id, title) };
}

function historyEntry(id: string, title: string, entryId = `entry-${id}`): PlaybackSessionQueueResponse["history"][number] {
  return { historyId: `history-${id}`, entryId, media: coreMedia(id, title) };
}

function playback(overrides: Partial<PlaybackResponse> = {}): PlaybackResponse {
  const value: PlaybackResponse = {
    sessionId: "session-1",
    nextEventSequence: 1,
    mediaGrant: { token: "grant", expiresAt: "2099-01-01T00:00:00Z" },
    continuationCredential: {
      token: "continuation",
      expiresAt: "2099-01-01T00:00:00Z",
      generation: 1,
      origin: "http://localhost:32500",
    },
    media: coreMedia("episode-1", "The Castle"),
    currentQueueEntryId: "entry-episode-1",
    sourceUrl: "/api/media/episode-1/stream.mp4",
    resources: [
      {
        id: "direct-original-en",
        sourceUrl: "/api/media/episode-1/stream.mp4",
        streamFormat: "direct",
        audioStreamId: "audio-1",
        subtitleMode: "off",
        default: true,
      },
    ],
    directPlay: true,
    streamFormat: "direct",
    decision: {
      mode: "direct_play",
      reason: "Browser compatible",
      reasonCodes: ["exact_tuple"],
      requiresTranscode: false,
      isProxied: true,
      isServerCached: false,
    },
    policy: {
      networkClass: "local",
      directPlayPolicy: "prefer",
      directStreamPolicy: "allow",
      transcodePolicy: "allow",
      allowHdr: true,
      serverClamps: [],
    },
    qualityOffers: {
      contractId: "PC-PLAYBACK", schemaVersion: "quality-offers.v1", mediaId: "episode-1",
      versionId: "qver-episode-1", sourceRevision: "qsrc-episode-1", offerRevision: "qrev-episode-1",
      offers: [
        { selectionId: "qsel-automatic", label: "Automatic", kind: "automatic" },
        { selectionId: "qsel-original", label: "Original Quality", kind: "original" },
      ],
    },
    qualitySelection: { mode: "automatic" },
    audioStreams: [
      {
        id: "audio-1",
        kind: "audio",
        codec: "aac",
        language: "en",
        displayTitle: "English",
      },
    ],
    selectedAudioStreamId: "audio-1",
    subtitleStreams: [],
    chapters: [],
    queue: [queueEntry("episode-2", "Palindrome")],
    repeatMode: "off",
    queueRevision: 7,
    playbackRevision: 1,
    timeline: {
      type: "vod",
      durationSeconds: 120,
      canPause: true,
      canSeek: true,
    },
    resumePositionSeconds: 12,
    generation: 1,
  };
  return { ...value, ...overrides };
}

function queue(
  overrides: Partial<PlaybackSessionQueueResponse> = {},
): PlaybackSessionQueueResponse {
  const value: PlaybackSessionQueueResponse = {
    sessionId: "session-1",
    current: queueEntry("episode-1", "The Castle"),
    items: [
      queueEntry("episode-2", "Palindrome"),
      queueEntry("episode-3", "The Myth of Sisyphus"),
    ],
    history: [historyEntry("episode-0", "Waiting for Dutch")],
    total: 2,
    canMutate: true,
    repeatMode: "off",
    revision: 7,
  };
  return { ...value, ...overrides };
}

function renderPlayer(
  source: FixturePorticoDataSource,
  mediaId = "episode-1",
  extra?: ReactNode,
) {
  return render(
    <DataProvider source={source} initialViewer={viewer}>
      <WebDisplayPreferencesProvider>
        <MemoryRouter initialEntries={[`/watch/${mediaId}`]}>
          <PlaybackSessionProvider>
            <Routes>
              <Route path="/watch/:id" element={<WatchPage />} />
              <Route path="*" element={null} />
            </Routes>
            <PlayerDock />
            {extra}
          </PlaybackSessionProvider>
        </MemoryRouter>
      </WebDisplayPreferencesProvider>
    </DataProvider>,
  );
}

function QueueMutationHarness() {
  const playback = usePlaybackSession();
  return (
    <>
      <button
        type="button"
        onClick={() => void playback.appendQueue(["episode-4"])}
      >
        Append fixture
      </button>
      <button
        type="button"
        onClick={() => void playback.playNext(["episode-5"])}
      >
        Play fixture next
      </button>
    </>
  );
}

function PlaybackRecoveryHarness() {
  const playback = usePlaybackSession();
  return (
    <>
      <button type="button" onClick={() => playback.fail("route")}>
        Fail route
      </button>
      <button
        type="button"
        onClick={() => void playback.start("episode-1", { startSeconds: 0 })}
      >
        Play from beginning
      </button>
    </>
  );
}

function PlaybackContractHarness() {
  const playback = usePlaybackSession();
  return (
    <>
      <button type="button" onClick={() => void playback.start("episode-2")}>Replace playback</button>
      <button type="button" onClick={() => void playback.renewGrant()}>Renew grant</button>
    </>
  );
}

function ReplacementTargetsHarness() {
  const player = usePlaybackSession();
  return <>
    <button type="button" onClick={() => void player.start("episode-2")}>Target media</button>
    <button type="button" onClick={() => void player.startLive("channel-2")}>Target live</button>
    <button type="button" onClick={() => void player.startDVR("recording-2")}>Target DVR</button>
    <button type="button" onClick={() => void player.startLibraryChannel("library-channel-2")}>Target library channel</button>
  </>;
}

function LateSourceActivityHarness() {
  const player = usePlaybackSession();
  return <>
    <button type="button" onClick={() => void player.touch({ state: "playing", positionSeconds: 40 })}>Send late progress</button>
    <button type="button" onClick={() => void player.renewGrant()}>Send late renewal</button>
  </>;
}

function CollapsedTrackStartHarness() {
  const playback = usePlaybackSession();
  return <button type="button" onClick={() => void playback.start("6b705dadddf08a29ee14be80ecf9b5cb01968814")}>Play track</button>;
}

function NavigateAwayHarness() {
  const navigate = useNavigate();
  return <button type="button" onClick={() => navigate('/settings/account')}>Open account settings</button>;
}

beforeEach(() => {
  hlsFixtures.instances.length = 0;
  localStorage.clear();
  vi.spyOn(HTMLMediaElement.prototype, "canPlayType").mockReturnValue(
    "probably",
  );
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(function load(
    this: HTMLMediaElement,
  ) {
    queueMicrotask(() => this.dispatchEvent(new Event("loadedmetadata")));
  });
  vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(function play(
    this: HTMLMediaElement,
  ) {
    Object.defineProperty(this, "paused", { configurable: true, value: false });
    this.dispatchEvent(new Event("play"));
    this.dispatchEvent(new Event("playing"));
    return Promise.resolve();
  });
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(
    function pause(this: HTMLMediaElement) {
      Object.defineProperty(this, "paused", {
        configurable: true,
        value: true,
      });
      this.dispatchEvent(new Event("pause"));
    },
  );
  Object.defineProperty(HTMLMediaElement.prototype, "duration", {
    configurable: true,
    get: () => 120,
  });
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  vi.restoreAllMocks();
});

describe("production playback surface", () => {
  it("reuses one immutable handoff request and terminal snapshot across duplicate consumers", () => {
    const terminal = { disposition: "completed" as const, positionSeconds: 120, durationSeconds: 120 };
    const first = immutablePlaybackHandoffIntent(undefined, "session-1", { preparedSessionId: "prepared-2", entryId: "entry-2" }, terminal);
    const duplicate = immutablePlaybackHandoffIntent(first, "session-1", { preparedSessionId: "prepared-2", entryId: "entry-2" }, { ...terminal });
    expect(first.request.requestId).toMatch(/^web-[0-9a-f-]{36}$/);
    expect(duplicate).toBe(first);
    expect(duplicate.request).toBe(first.request);
    expect(Object.isFrozen(first.request)).toBe(true);
    expect(Object.isFrozen(first.request.previousTerminal)).toBe(true);
    expect(immutablePlaybackHandoffIntent(first, "session-1", { preparedSessionId: "prepared-3", entryId: "entry-3" }, terminal).request.requestId).not.toBe(first.request.requestId);
  });
  it("distinguishes ambiguous handoff transport failures from definitive contract rejections", () => {
    expect(isAmbiguousPlaybackMutationFailure(new TypeError("network failed"))).toBe(true);
    expect(isAmbiguousPlaybackMutationFailure(new ApiError(503, "unavailable", "Retry exactly."))).toBe(true);
    expect(isAmbiguousPlaybackMutationFailure(new ApiError(401, "session_refresh_required", "Reconcile exactly."))).toBe(true);
    expect(isAmbiguousPlaybackMutationFailure(new ApiError(404, "session_not_found", "Do not infer success."))).toBe(true);
    expect(isAmbiguousPlaybackMutationFailure(new ApiError(409, "handoff_in_progress", "Reconcile exactly."))).toBe(true);
    expect(isAmbiguousPlaybackMutationFailure(new ApiError(409, "prepared_handoff_in_progress", "Reconcile exactly."))).toBe(true);
    expect(isAmbiguousPlaybackMutationFailure(new ApiError(409, "temporarily_blocked", "Retry exactly.", undefined, { retryable: true }))).toBe(true);
    expect(isAmbiguousPlaybackMutationFailure(new ApiError(409, "stale_event", "Rejected."))).toBe(false);
  });

  it("drains the durable terminal outbox before restoring a replacement session", async () => {
    const source = new FixturePorticoDataSource();
    const replacement = playback({ sessionId: "session-recovered", media: coreMedia("episode-2", "Palindrome"), currentQueueEntryId: "entry-episode-2" });
    const recover = vi.spyOn(source as PorticoDataSource, "recoverPendingPlaybackTerminals").mockResolvedValue(undefined);
    const restore = vi.spyOn(source as PorticoDataSource, "restorePlayback").mockResolvedValue({ active: true, playback: replacement });

    render(
      <DataProvider source={source} initialViewer={viewer}>
        <WebDisplayPreferencesProvider>
          <MemoryRouter initialEntries={["/"]}>
            <PlaybackSessionProvider><PlayerDock /></PlaybackSessionProvider>
          </MemoryRouter>
        </WebDisplayPreferencesProvider>
      </DataProvider>,
    );

    await waitFor(() => expect(restore).toHaveBeenCalled());
    expect(recover).toHaveBeenCalledWith(expect.any(AbortSignal));
    expect(recover.mock.invocationCallOrder[0]).toBeLessThan(restore.mock.invocationCallOrder[0]);
    expect(await screen.findByLabelText("Now playing Palindrome")).toBeInTheDocument();
    expect(screen.queryByLabelText("Now playing The Castle")).not.toBeInTheDocument();
  });

  it("drains a committed stop before an inactive restore and keeps playback closed", async () => {
    const source = new FixturePorticoDataSource();
    const recover = vi.spyOn(source as PorticoDataSource, "recoverPendingPlaybackTerminals").mockResolvedValue(undefined);
    const restore = vi.spyOn(source as PorticoDataSource, "restorePlayback").mockResolvedValue({ active: false });

    render(
      <DataProvider source={source} initialViewer={viewer}>
        <WebDisplayPreferencesProvider>
          <MemoryRouter initialEntries={["/"]}>
            <PlaybackSessionProvider><PlayerDock /></PlaybackSessionProvider>
          </MemoryRouter>
        </WebDisplayPreferencesProvider>
      </DataProvider>,
    );

    await waitFor(() => expect(restore).toHaveBeenCalled());
    expect(recover.mock.invocationCallOrder[0]).toBeLessThan(restore.mock.invocationCallOrder[0]);
    expect(screen.queryByLabelText(/^Now playing /)).not.toBeInTheDocument();
  });
  it("shows a recoverable dock failure when collapsed track preparation is rejected", async () => {
    const source = new FixturePorticoDataSource();
    const start = vi.spyOn(source as PorticoDataSource, "startPlayback").mockRejectedValue(new ApiError(422, "delivery_policy_unsatisfied", "No legal playback plan."));
    render(
      <DataProvider source={source} initialViewer={viewer}>
        <WebDisplayPreferencesProvider>
          <MemoryRouter initialEntries={["/media/6b705dadddf08a29ee14be80ecf9b5cb01968814"]}>
            <PlaybackSessionProvider>
              <PlayerDock />
              <CollapsedTrackStartHarness />
            </PlaybackSessionProvider>
          </MemoryRouter>
        </WebDisplayPreferencesProvider>
      </DataProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Play track" }));
    const failure = await screen.findByRole("alert");
    expect(failure).toHaveTextContent("Playback could not start");
    expect(failure.closest(".player-mini")).toHaveClass("player-pending-shell");
    expect(start).toHaveBeenCalledWith("6b705dadddf08a29ee14be80ecf9b5cb01968814", expect.any(Object), expect.any(AbortSignal));
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Play" })).toBeDisabled();
    expect(screen.getAllByRole("button", { name: "Close player" }).length).toBeGreaterThan(0);
  });

  it("mounts the full player chrome while preparing and preserves it around a start failure", async () => {
    const source = new FixturePorticoDataSource();
    let rejectStart: ((reason: unknown) => void) | undefined;
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockImplementation(() => new Promise((_, reject) => { rejectStart = reject; }));
    renderPlayer(source);

    const preparing = await screen.findByRole("status");
    expect(preparing).toHaveTextContent("Preparing playback");
    expect(preparing.closest(".player-full")).toHaveClass("player-pending-shell");
    expect(preparing.closest(".player-full")?.querySelector('input[type="range"]')).toBeDisabled();
    expect(screen.getByRole("button", { name: "Play" })).toBeDisabled();
    expect(screen.getAllByRole("button", { name: "Close player" }).length).toBeGreaterThan(0);

    await waitFor(() => expect(rejectStart).toBeTypeOf("function"));
    rejectStart?.(new ApiError(422, "delivery_policy_unsatisfied", "No legal playback plan."));
    const failure = await screen.findByRole("alert");
    expect(failure).toHaveTextContent("Playback could not start");
    expect(failure.closest(".player-full")).toHaveClass("player-pending-shell");
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Play" })).toBeDisabled();
  });

  it("dismisses a handled playback failure when the user navigates elsewhere", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockRejectedValue(new ApiError(422, "delivery_policy_unsatisfied", "No legal playback plan."));
    render(
      <DataProvider source={source} initialViewer={viewer}>
        <WebDisplayPreferencesProvider>
          <MemoryRouter initialEntries={["/media/track"]}>
            <PlaybackSessionProvider><PlayerDock /><CollapsedTrackStartHarness /><NavigateAwayHarness /></PlaybackSessionProvider>
          </MemoryRouter>
        </WebDisplayPreferencesProvider>
      </DataProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Play track" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Playback could not start");
    fireEvent.click(screen.getByRole("button", { name: "Open account settings" }));
    await waitFor(() => expect(screen.queryByText("Playback could not start")).not.toBeInTheDocument());
  });

  it("keeps the active session until replacement is prepared and coalesces grant renewal", async () => {
    const source = new FixturePorticoDataSource();
    const start = vi.spyOn(source as PorticoDataSource, "startPlayback")
      .mockResolvedValueOnce(playback())
      .mockRejectedValueOnce(new Error("replacement unavailable"));
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);
    let finishRenewal: ((value: {token: string; expiresAt: string}) => void) | undefined;
    const renew = vi.spyOn(source as PorticoDataSource, "renewPlaybackMediaGrant").mockImplementation(
      () => new Promise((resolve) => { finishRenewal = resolve; }),
    );

    renderPlayer(source, "episode-1", <PlaybackContractHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Replace playback" }));
    await waitFor(() => expect(start).toHaveBeenCalledTimes(2));
    expect(stop).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Now playing The Castle")).toBeInTheDocument();

    const renewal = screen.getByRole("button", { name: "Renew grant" });
    fireEvent.click(renewal);
    fireEvent.click(renewal);
    await waitFor(() => expect(renew).toHaveBeenCalledOnce());
    finishRenewal?.({token: "renewed", expiresAt: "2099-01-02T00:00:00Z"});
  });

  it("routes media, Live TV, DVR, and Library Channel replacements through one atomic Core adapter", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockImplementation(async (sessionId) => queue({ sessionId, revision: 11 }));
    const replacements = [
      playback({ sessionId: "session-media", media: coreMedia("episode-2", "Media target"), playbackRevision: 2, queueRevision: 11 }),
      playback({ sessionId: "session-live", media: coreMedia("channel-2", "Live target"), isLive: true, playbackRevision: 3, queueRevision: 11 }),
      playback({ sessionId: "session-dvr", media: coreMedia("recording-2", "DVR target"), playbackRevision: 4, queueRevision: 11 }),
      playback({ sessionId: "session-library", media: coreMedia("library-channel-2", "Library target"), isLive: true, playbackRevision: 5, queueRevision: 11 }),
    ];
    const replace = vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget")
      .mockResolvedValueOnce({ outcome: "accepted", value: replacements[0] })
      .mockResolvedValueOnce({ outcome: "accepted", value: replacements[1] })
      .mockResolvedValueOnce({ outcome: "accepted", value: replacements[2] })
      .mockResolvedValueOnce({ outcome: "accepted", value: { sourceType: "library-channel", playback: replacements[3] } as never });
    const rawLive = vi.spyOn(source as PorticoDataSource, "startLiveTVPlayback");
    const rawDVR = vi.spyOn(source as PorticoDataSource, "startDVRPlayback");
    const rawLibrary = vi.spyOn(source as PorticoDataSource, "startLibraryChannelPlayback");
    renderPlayer(source, "episode-1", <ReplacementTargetsHarness />);
    await screen.findByLabelText("Now playing The Castle");

    for (const name of ["Target media", "Target live", "Target DVR", "Target library channel"]) {
      fireEvent.click(screen.getByRole("button", { name }));
      await waitFor(() => expect(replace).toHaveBeenCalledTimes(["Target media", "Target live", "Target DVR", "Target library channel"].indexOf(name) + 1));
    }

    expect(replace.mock.calls.map(([target]) => target.kind)).toEqual(["media", "live-tv", "dvr", "library-channel"]);
    expect(replace.mock.calls[0][1]).toEqual(expect.objectContaining({
      sourceSessionId: "session-1",
      previousTerminal: { disposition: "stopped", positionSeconds: 12, durationSeconds: 120 },
      expectedQueueRevision: 11,
      expectedPlaybackRevision: 1,
    }));
    expect(rawLive).not.toHaveBeenCalled();
    expect(rawDVR).not.toHaveBeenCalled();
    expect(rawLibrary).not.toHaveBeenCalled();
    expect(await screen.findByLabelText("Now playing Library target")).toBeInTheDocument();
  });

  it("retains usable current playback on a definitive replacement rejection", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget").mockResolvedValue({
      outcome: "source-retained",
      sourceSessionId: "session-1",
      rejection: { status: 422, code: "delivery_policy_unsatisfied", detail: "The requested target has no legal playback plan." },
    });
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback");
    renderPlayer(source, "episode-1", <ReplacementTargetsHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Target media" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The requested target has no legal playback plan.");
    expect(screen.getByLabelText("Now playing The Castle")).toBeInTheDocument();
    expect(stop).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });

  it("clears old playback authority when Core reports the source closed", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget").mockResolvedValue({
      outcome: "source-closed",
      sourceSessionId: "session-1",
      rejection: { status: 409, code: "replacement_failed_after_close", detail: "The old session closed before the target could be exposed." },
      terminal: {} as never,
    });
    renderPlayer(source, "episode-1", <ReplacementTargetsHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Target media" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The old session closed before the target could be exposed.");
    expect(screen.queryByLabelText("Now playing The Castle")).not.toBeInTheDocument();
  });

  it("treats source-inactive as proof old authority is gone and fences late activity", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    const touch = vi.spyOn(source as PorticoDataSource, "touchPlayback");
    const renew = vi.spyOn(source as PorticoDataSource, "renewPlaybackMediaGrant");
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback");
    vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget").mockResolvedValue({
      outcome: "source-inactive",
      sourceSessionId: "session-1",
      rejection: { status: 409, code: "playback_source_inactive", detail: "The old playback actor is no longer active." },
    });
    renderPlayer(source, "episode-1", <><ReplacementTargetsHarness /><LateSourceActivityHarness /></>);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Target media" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The old playback actor is no longer active.");
    expect(screen.queryByLabelText("Now playing The Castle")).not.toBeInTheDocument();
    const progressCalls = touch.mock.calls.length;
    fireEvent.click(screen.getByRole("button", { name: "Send late progress" }));
    fireEvent.click(screen.getByRole("button", { name: "Send late renewal" }));
    await Promise.resolve();
    expect(touch).toHaveBeenCalledTimes(progressCalls);
    expect(renew).not.toHaveBeenCalled();
    expect(stop).not.toHaveBeenCalled();
  });

  it("starts a queued newer target fresh after source-inactive reconciliation", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockImplementation(async (sessionId) => queue({ sessionId }));
    let resolveReplacement!: (outcome: { outcome: "source-inactive"; sourceSessionId: string; rejection: { status: number; code: string; detail: string } }) => void;
    const inactive = new Promise<{ outcome: "source-inactive"; sourceSessionId: string; rejection: { status: number; code: string; detail: string } }>((resolve) => { resolveReplacement = resolve; });
    const replace = vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget").mockImplementationOnce(() => inactive);
    const latest = playback({ sessionId: "session-fresh-live", media: coreMedia("channel-2", "Fresh live target"), isLive: true });
    const rawLive = vi.spyOn(source as PorticoDataSource, "startLiveTVPlayback").mockResolvedValue(latest);
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback");
    renderPlayer(source, "episode-1", <ReplacementTargetsHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Target media" }));
    await waitFor(() => expect(replace).toHaveBeenCalledOnce());
    fireEvent.click(screen.getByRole("button", { name: "Target live" }));
    resolveReplacement({
      outcome: "source-inactive",
      sourceSessionId: "session-1",
      rejection: { status: 409, code: "playback_source_inactive", detail: "Source is gone." },
    });

    await screen.findByLabelText("Now playing Fresh live target");
    expect(replace).toHaveBeenCalledOnce();
    expect(rawLive).toHaveBeenCalledWith("channel-2", expect.any(AbortSignal));
    expect(stop).not.toHaveBeenCalled();
  });

  it("restores and exact-verifies a committed replacement before adopting it", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    const committed = { outcome: "committed-restore-required" as const, sourceSessionId: "session-1", replacementSessionId: "session-restored" };
    vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget").mockResolvedValue(committed);
    const restored = playback({ sessionId: "session-restored", media: coreMedia("episode-2", "Restored target") });
    const restore = vi.spyOn(source as PorticoDataSource, "restoreCommittedPlaybackReplacement").mockResolvedValue(restored);
    renderPlayer(source, "episode-1", <ReplacementTargetsHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Target media" }));

    await screen.findByLabelText("Now playing Restored target");
    expect(restore).toHaveBeenCalledWith(committed, expect.any(Object), expect.any(AbortSignal));
  });

  it("keeps an ambiguous replacement fenced and exact-retries it without stop-then-start", async () => {
    const source = new FixturePorticoDataSource();
    const rawStart = vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    const replace = vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget").mockRejectedValue(new TypeError("replacement response lost"));
    const recovered = playback({ sessionId: "session-retried", media: coreMedia("episode-2", "Retried target") });
    const retryPending = vi.spyOn(source as PorticoDataSource, "retryPendingPlaybackTerminalMutation").mockResolvedValue({ outcome: "accepted", value: recovered });
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback");
    renderPlayer(source, "episode-1", <ReplacementTargetsHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Target media" }));
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    await screen.findByLabelText("Now playing Retried target");
    expect(retryPending).toHaveBeenCalledWith("session-1", expect.any(AbortSignal));
    expect(replace).toHaveBeenCalledOnce();
    expect(rawStart).toHaveBeenCalledOnce();
    expect(stop).not.toHaveBeenCalled();
  });

  it("reconciles an ambiguous target before atomically honoring a different later target", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockImplementation(async (sessionId) => queue({ sessionId }));
    const reconciled = playback({ sessionId: "session-reconciled", media: coreMedia("episode-2", "Reconciled media") });
    const latest = playback({ sessionId: "session-latest", media: coreMedia("channel-2", "Latest live target"), isLive: true });
    const replace = vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget")
      .mockRejectedValueOnce(new TypeError("replacement response lost"))
      .mockResolvedValueOnce({ outcome: "accepted", value: latest });
    const retryPending = vi.spyOn(source as PorticoDataSource, "retryPendingPlaybackTerminalMutation")
      .mockResolvedValue({ outcome: "accepted", value: reconciled });
    const rawLive = vi.spyOn(source as PorticoDataSource, "startLiveTVPlayback");
    renderPlayer(source, "episode-1", <ReplacementTargetsHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Target media" }));
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "Target live" }));

    await screen.findByLabelText("Now playing Latest live target");
    expect(retryPending).toHaveBeenCalledWith("session-1", expect.any(AbortSignal));
    expect(replace.mock.calls.map(([target]) => target.kind)).toEqual(["media", "live-tv"]);
    expect(replace.mock.calls[1][1]).toEqual(expect.objectContaining({ sourceSessionId: "session-reconciled" }));
    expect(rawLive).not.toHaveBeenCalled();
  });

  it("preserves committed replacement identity across restore failure before a later target", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockImplementation(async (sessionId) => queue({ sessionId }));
    const committed = { outcome: "committed-restore-required" as const, sourceSessionId: "session-1", replacementSessionId: "session-restored" };
    const restored = playback({ sessionId: "session-restored", media: coreMedia("episode-2", "Exactly restored") });
    const latest = playback({ sessionId: "session-after-restore", media: coreMedia("channel-2", "Live after restore"), isLive: true });
    const replace = vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget")
      .mockResolvedValueOnce(committed)
      .mockResolvedValueOnce({ outcome: "accepted", value: latest });
    const restore = vi.spyOn(source as PorticoDataSource, "restoreCommittedPlaybackReplacement")
      .mockRejectedValueOnce(new ApiError(409, "playback_replacement_restore_mismatch", "Active restore did not match."))
      .mockResolvedValueOnce(restored);
    renderPlayer(source, "episode-1", <ReplacementTargetsHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Target media" }));
    await screen.findByRole("alert");
    expect(screen.queryByLabelText("Now playing The Castle")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Target live" }));

    await screen.findByLabelText("Now playing Live after restore");
    expect(restore).toHaveBeenCalledTimes(2);
    expect(restore.mock.calls[0][0]).toEqual(committed);
    expect(restore.mock.calls[1][0]).toEqual(committed);
    expect(replace.mock.calls[1][1]).toEqual(expect.objectContaining({ sourceSessionId: "session-restored" }));
  });

  it("serializes a rapid later target behind the in-flight atomic replacement", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockImplementation(async (sessionId) => queue({ sessionId }));
    const first = playback({ sessionId: "session-first", media: coreMedia("episode-2", "First target") });
    const second = playback({ sessionId: "session-rapid-second", media: coreMedia("channel-2", "Rapid second target"), isLive: true });
    const latest = playback({ sessionId: "session-rapid-latest", media: coreMedia("recording-2", "Rapid latest target") });
    let resolveFirst!: (outcome: { outcome: "accepted"; value: PlaybackResponse }) => void;
    const firstOutcome = new Promise<{ outcome: "accepted"; value: PlaybackResponse }>((resolve) => { resolveFirst = resolve; });
    const replace = vi.spyOn(source as PorticoDataSource, "replacePlaybackTarget")
      .mockImplementationOnce((_target, _input, signal) => {
        expect(signal.aborted).toBe(false);
        return firstOutcome;
      })
      .mockResolvedValueOnce({ outcome: "accepted", value: second })
      .mockResolvedValueOnce({ outcome: "accepted", value: latest });
    renderPlayer(source, "episode-1", <ReplacementTargetsHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Target media" }));
    await waitFor(() => expect(replace).toHaveBeenCalledOnce());
    fireEvent.click(screen.getByRole("button", { name: "Target live" }));
    fireEvent.click(screen.getByRole("button", { name: "Target DVR" }));
    expect(replace).toHaveBeenCalledOnce();
    resolveFirst({ outcome: "accepted", value: first });

    await screen.findByLabelText("Now playing Rapid latest target");
    expect(replace).toHaveBeenCalledTimes(3);
    expect(replace.mock.calls[1][1]).toEqual(expect.objectContaining({ sourceSessionId: "session-first" }));
    expect(replace.mock.calls[2][1]).toEqual(expect.objectContaining({ sourceSessionId: "session-rapid-second" }));
  });

  it("rebuilds a managed HLS adapter after manual route retry without blindly loading the media element", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue(
      "Mozilla/5.0 Chrome/147.0.0.0 Safari/537.36",
    );
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback({
        streamFormat: "hls",
        sourceUrl: "/api/playback-resources/hls-original-en/index.m3u8",
        resources: [
          {
            id: "hls-original-en",
            sourceUrl: "/api/playback-resources/hls-original-en/index.m3u8",
            streamFormat: "hls",
            audioStreamId: "audio-1",
            subtitleMode: "off",
            default: true,
          },
        ],
      }),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());
    const renew = vi
      .spyOn(source as PorticoDataSource, "renewPlaybackMediaGrant")
      .mockResolvedValue({
        token: "renewed-grant",
        expiresAt: "2099-01-01T00:00:00Z",
      });

    renderPlayer(source, "episode-1", <PlaybackRecoveryHarness />);
    await screen.findByLabelText("Now playing The Castle");
    await waitFor(() => expect(hlsFixtures.instances).toHaveLength(1));
    const credentialedRequest = { withCredentials: false } as XMLHttpRequest;
    hlsFixtures.instances[0].config.xhrSetup?.(credentialedRequest);
    expect(credentialedRequest.withCredentials).toBe(true);
    const loadCalls = vi.mocked(HTMLMediaElement.prototype.load).mock.calls
      .length;
    fireEvent.click(screen.getByRole("button", { name: "Fail route" }));
    fireEvent.click(await screen.findByRole("button", { name: "Try again" }));

    await waitFor(() =>
      expect(renew).toHaveBeenCalledWith("session-1", expect.any(AbortSignal)),
    );
    await waitFor(() => expect(hlsFixtures.instances).toHaveLength(2));
    expect(hlsFixtures.instances[0].destroy).toHaveBeenCalledOnce();
    expect(HTMLMediaElement.prototype.load).toHaveBeenCalledTimes(loadCalls);
  });

  it("seeks and resumes an active item when Play from beginning targets the same media", async () => {
    const source = new FixturePorticoDataSource();
    const start = vi
      .spyOn(source as PorticoDataSource, "startPlayback")
      .mockResolvedValue(playback());
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());

    renderPlayer(source, "episode-1", <PlaybackRecoveryHarness />);
    const surface = await screen.findByLabelText("Now playing The Castle");
    const media = surface.querySelector("video") as HTMLVideoElement;
    media.currentTime = 71;
    await waitFor(() =>
      expect(HTMLMediaElement.prototype.play).toHaveBeenCalled(),
    );
    vi.mocked(HTMLMediaElement.prototype.play).mockClear();
    fireEvent.click(
      screen.getByRole("button", { name: "Play from beginning" }),
    );

    await waitFor(() => expect(media.currentTime).toBe(0));
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledOnce();
    expect(start).toHaveBeenCalledOnce();
  });

  it("only classifies an explicit pre-play manifest 404 or 410 as a missing source", () => {
    expect(isExplicitMissingHLSManifest(404, "manifestLoadError", false)).toBe(
      true,
    );
    expect(isExplicitMissingHLSManifest(410, "manifestLoadError", false)).toBe(
      true,
    );
    expect(isExplicitMissingHLSManifest(404, "fragLoadError", false)).toBe(
      false,
    );
    expect(isExplicitMissingHLSManifest(503, "manifestLoadError", false)).toBe(
      false,
    );
    expect(isExplicitMissingHLSManifest(404, "manifestLoadError", true)).toBe(
      false,
    );
  });

  it("retries only reviewed pre-play manifest publication waits", () => {
    expect(isTransientHLSManifestWait(409, "manifestLoadError", false)).toBe(true);
    expect(isTransientHLSManifestWait(425, "manifestLoadError", false)).toBe(true);
    expect(isTransientHLSManifestWait(503, "manifestLoadError", false)).toBe(true);
    expect(isTransientHLSManifestWait(503, "fragLoadError", false)).toBe(false);
    expect(isTransientHLSManifestWait(503, "manifestLoadError", true)).toBe(false);
    expect(isTransientHLSManifestWait(500, "manifestLoadError", false)).toBe(false);
  });

  it("uses native HLS only in Safari even when Chromium reports tentative support", () => {
    expect(shouldUseNativeHLS("maybe", "Mozilla/5.0 Safari/605.1.15")).toBe(
      true,
    );
    expect(
      shouldUseNativeHLS(
        "probably",
        "Mozilla/5.0 Chrome/141.0.0.0 Safari/537.36",
      ),
    ).toBe(false);
    expect(
      shouldUseNativeHLS("maybe", "Mozilla/5.0 Edg/141.0.0.0 Safari/537.36"),
    ).toBe(false);
    expect(shouldUseNativeHLS("", "Mozilla/5.0 Safari/605.1.15")).toBe(false);
  });

  it("removes the API off sentinel from playable subtitle choices", () => {
    const sentinel = {
      id: "sub_none",
      kind: "subtitle",
      codec: "",
      language: "",
      displayTitle: "None",
    } as PlaybackResponse["subtitleStreams"][number];
    const english = {
      id: "subtitle-en",
      kind: "subtitle",
      codec: "srt",
      language: "en",
      displayTitle: "English",
    } as PlaybackResponse["subtitleStreams"][number];
    expect(playableSubtitleStreams([sentinel, english])).toEqual([english]);
  });

  it("keeps the complete control contract across the full and docked layouts", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback(),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());

    renderPlayer(source);

    const surface = await screen.findByLabelText("Now playing The Castle");
    expect(surface).toHaveClass("player-full");
    expect(surface.querySelector(".player-copy-art")).toHaveAttribute(
      "src",
      "http://localhost:3000/poster.jpg",
    );
    const controlLabels = [
      "Previous item",
      "Rewind 10 seconds",
      "Pause",
      "Forward 30 seconds",
      "Next item",
      "Volume",
      "Subtitles",
      "Playback settings",
      "Queue",
      "Fullscreen",
      "Close player",
    ];
    for (const label of controlLabels) {
      await waitFor(() =>
        expect(
          surface.querySelector(`[aria-label="${label}"]`),
        ).toBeInTheDocument(),
      );
    }

    fireEvent.click(screen.getByLabelText("Collapse player"));
    await waitFor(() => expect(surface).toHaveClass("player-mini"));
    for (const label of controlLabels) {
      expect(
        surface.querySelector(`[aria-label="${label}"]`),
      ).toBeInTheDocument();
    }
    expect(screen.getByLabelText("Playback position")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Expand player" }));
    await waitFor(() => expect(surface).toHaveClass("player-full"));
  });

  it("keeps one media element alive while collapsing and expanding the player", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback(),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());

    renderPlayer(source);
    const fullSurface = await screen.findByLabelText("Now playing The Castle");
    const media = fullSurface.querySelector("video");
    expect(media).toBeInTheDocument();
    await waitFor(() =>
      expect(HTMLMediaElement.prototype.play).toHaveBeenCalled(),
    );
    const playCalls = vi.mocked(HTMLMediaElement.prototype.play).mock.calls
      .length;

    fireEvent.click(screen.getByRole("button", { name: "Collapse player" }));
    const dockedSurface = await screen.findByLabelText(
      "Now playing The Castle",
    );
    expect(dockedSurface).toHaveClass("player-mini");
    expect(dockedSurface.querySelector("video")).toBe(media);
    fireEvent.click(
      within(dockedSurface).getByRole("button", { name: "Expand player" }),
    );
    const expandedSurface = await screen.findByLabelText(
      "Now playing The Castle",
    );
    expect(expandedSurface).toHaveClass("player-full");
    expect(expandedSurface.querySelector("video")).toBe(media);
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(playCalls);
  });

  it("starts a canonical session and seeks with the real media element", async () => {
    const source = new FixturePorticoDataSource();
    const start = vi
      .spyOn(source as PorticoDataSource, "startPlayback")
      .mockResolvedValue(playback());
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());
    const touch = vi
      .spyOn(source as PorticoDataSource, "touchPlayback")
      .mockResolvedValue({
        accepted: true,
        duplicate: false,
        stale: false,
        generation: 1,
        highestEventSequence: 1,
        sessionState: "playing",
      });

    renderPlayer(source);

    expect(
      await screen.findByLabelText("Now playing The Castle"),
    ).toBeInTheDocument();
    expect(start).toHaveBeenCalledWith(
      "episode-1",
      expect.objectContaining({
        intent: expect.objectContaining({
          transportClass: "unknown",
          quality: { mode: "automatic" },
        }),
      }),
      expect.any(AbortSignal),
    );
    await waitFor(() =>
      expect(HTMLMediaElement.prototype.play).toHaveBeenCalled(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Forward 30 seconds" }));
    await waitFor(() =>
      expect(touch).toHaveBeenCalledWith(
        "session-1",
        expect.objectContaining({ positionSeconds: 42 }),
        expect.any(AbortSignal),
        false,
      ),
    );
  });

  it("preserves the canonical resume marker when the queue refreshes before media metadata", async () => {
    const source = new FixturePorticoDataSource();
    const resumedPlayback = playback({ resumePositionSeconds: 47 });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      resumedPlayback,
    );
    let resolveQueue!: (value: PlaybackSessionQueueResponse) => void;
    const queueResponse = new Promise<PlaybackSessionQueueResponse>(
      (resolve) => {
        resolveQueue = resolve;
      },
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockReturnValue(queueResponse);
    vi.mocked(HTMLMediaElement.prototype.load).mockImplementation(
      () => undefined,
    );

    renderPlayer(source);

    const surface = await screen.findByLabelText("Now playing The Castle");
    const media = surface.querySelector("video") as HTMLVideoElement;
    await waitFor(() =>
      expect(media.src).toContain("/api/media/episode-1/stream.mp4"),
    );
    expect(media.currentTime).toBe(0);
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();

    // A queue-only response must not replace the source generation while this
    // same session is still waiting for loadedmetadata.
    resolveQueue(queue());
    await waitFor(() =>
      expect(HTMLMediaElement.prototype.load).toHaveBeenCalledTimes(1),
    );
    expect(media.currentTime).toBe(0);
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();

    fireEvent.loadedMetadata(media);
    expect(media.currentTime).toBe(47);
    await waitFor(() =>
      expect(HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(1),
    );
  });

  it("keeps expanded routing separate from browser fullscreen state", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback(),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());
    let fullscreenElement: Element | null = null;
    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      get: () => fullscreenElement,
    });
    Object.defineProperty(document, "exitFullscreen", {
      configurable: true,
      value: vi.fn(async () => {
        fullscreenElement = null;
        document.dispatchEvent(new Event("fullscreenchange"));
      }),
    });

    renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    Object.defineProperty(surface, "requestFullscreen", {
      configurable: true,
      value: vi.fn(async () => {
        fullscreenElement = surface;
        document.dispatchEvent(new Event("fullscreenchange"));
      }),
    });

    fireEvent.click(screen.getByRole("button", { name: "Fullscreen" }));
    await screen.findByRole("button", { name: "Exit fullscreen" });
    expect(surface).toHaveClass("player-full");
    fireEvent.click(screen.getByRole("button", { name: "Exit fullscreen" }));
    await screen.findByRole("button", { name: "Fullscreen" });
    expect(surface).toHaveClass("player-full");

    fireEvent.click(screen.getByRole("button", { name: "Collapse player" }));
    await waitFor(() => expect(surface).toHaveClass("player-mini"));
    fireEvent.click(screen.getByRole("button", { name: "Fullscreen" }));
    await waitFor(() => expect(surface).toHaveClass("player-full"));
    fireEvent.click(screen.getByRole("button", { name: "Exit fullscreen" }));
    await waitFor(() => expect(surface).toHaveClass("player-mini"));
  });

  it("mutates the real session queue and tears down playback on close", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback(),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());
    vi.spyOn(source as PorticoDataSource, "touchPlayback").mockResolvedValue({
      accepted: true,
      duplicate: false,
      stale: false,
      generation: 1,
      highestEventSequence: 1,
      sessionState: "paused",
    });
    const mutate = vi
      .spyOn(source as PorticoDataSource, "mutatePlaybackSessionQueue")
      .mockResolvedValue(queue());
    const stop = vi
      .spyOn(source as PorticoDataSource, "stopPlayback")
      .mockResolvedValue(undefined);

    renderPlayer(source);
    await screen.findByLabelText("Now playing The Castle");
    const queueTrigger = screen.getByLabelText("Queue");
    expect(queueTrigger).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(queueTrigger);
    const queueDialog = await screen.findByRole("dialog", { name: "Queue" });
    expect(queueTrigger).toHaveAttribute("aria-expanded", "true");
    expect(within(queueDialog).getByText("Now playing")).toBeInTheDocument();
    expect(within(queueDialog).getByText("The Castle")).toBeInTheDocument();
    fireEvent.click(
      within(queueDialog).getByRole("button", {
        name: "Move Palindrome later",
      }),
    );
    await waitFor(() =>
      expect(mutate).toHaveBeenCalledWith(
        "session-1",
        {
          action: "reorder",
          destinationEntryId: "entry-episode-3",
          entryId: "entry-episode-2",
          expectedRevision: 7,
          idempotencyKey: expect.any(String),
          placement: "after",
        },
        expect.any(AbortSignal),
      ),
    );
    fireEvent.click(within(queueDialog).getByRole("button", { name: "Shuffle" }));
    await waitFor(() =>
      expect(mutate).toHaveBeenCalledWith(
        "session-1",
        {
          action: "shuffle",
          expectedRevision: 7,
          idempotencyKey: expect.any(String),
        },
        expect.any(AbortSignal),
      ),
    );
    expect(mutate).toHaveBeenCalledTimes(2);
    fireEvent.click(
      within(queueDialog).getByRole("button", { name: "Close queue" }),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Queue" }),
      ).not.toBeInTheDocument(),
    );
    expect(queueTrigger).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(screen.getByRole("button", { name: "Close player" }));
    await waitFor(() =>
      expect(stop).toHaveBeenCalledWith(
        "session-1",
        expect.objectContaining({
          disposition: "stopped",
          positionSeconds: expect.any(Number),
          durationSeconds: expect.any(Number),
        }),
        expect.any(AbortSignal),
        true,
      ),
    );
    expect(
      screen.queryByLabelText("Now playing The Castle"),
    ).not.toBeInTheDocument();
  });

  it("sends stop without waiting for final progress during explicit close", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    vi.spyOn(source as PorticoDataSource, "touchPlayback").mockImplementation(
      () => new Promise(() => undefined),
    );
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockImplementation(
      () => new Promise(() => undefined),
    );

    renderPlayer(source);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Close player" }));
    await waitFor(() => expect(stop).toHaveBeenCalledOnce());
    expect(screen.queryByLabelText("Now playing The Castle")).not.toBeInTheDocument();
  });

  it("completes a finite VOD overrun without assigning or playing the HLS endpoint", async () => {
    const source = new FixturePorticoDataSource();
    const finitePlayback = playback({
      queue: [],
      repeatMode: "off",
      streamFormat: "hls",
      sourceUrl: "/api/playback-resources/final.m3u8",
      timeline: { type: "vod", durationSeconds: 60, canPause: true, canSeek: true },
    });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(finitePlayback);
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue({ items: [], total: 0, repeatMode: "off" }));
    const progress = vi.spyOn(source as PorticoDataSource, "touchPlayback");
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);

    const view = renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    const media = surface.querySelector("video") as HTMLVideoElement;
    Object.defineProperty(media, "duration", { configurable: true, value: 60 });
    media.currentTime = 46.2;
    vi.mocked(HTMLMediaElement.prototype.play).mockClear();

    fireEvent.click(screen.getByRole("button", { name: "Forward 30 seconds" }));

    expect(media.currentTime).toBe(46.2);
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
    expect(await screen.findByRole("region", { name: "Playback is complete." })).toBeInTheDocument();
    expect(screen.queryByLabelText("The Castle", { selector: "video" })).not.toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Playback position" })).toHaveValue("60");
    fireEvent.timeUpdate(media, { target: { currentTime: 0.7 } });
    expect(screen.getByRole("slider", { name: "Playback position" })).toHaveValue("60");
    expect(progress).not.toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({ completed: true }),
      expect.anything(),
      expect.anything(),
    );
    await waitFor(() => expect(stop).toHaveBeenCalledWith(
      "session-1",
      { disposition: "completed", positionSeconds: 60, durationSeconds: 60 },
      expect.any(AbortSignal),
      true,
    ));
    view.unmount();
  });

  it("appends and inserts next through revision-safe queue mutations", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback(),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());
    const mutate = vi
      .spyOn(source as PorticoDataSource, "mutatePlaybackSessionQueue")
      .mockResolvedValueOnce(queue({ revision: 8 }))
      .mockResolvedValueOnce(queue({ revision: 9 }));

    renderPlayer(source, "episode-1", <QueueMutationHarness />);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Append fixture" }));
    await waitFor(() =>
      expect(mutate).toHaveBeenCalledWith(
        "session-1",
        { action: "append", expectedRevision: 7, idempotencyKey: expect.any(String), mediaIds: ["episode-4"] },
        expect.any(AbortSignal),
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "Play fixture next" }));
    await waitFor(() =>
      expect(mutate).toHaveBeenLastCalledWith(
        "session-1",
        { action: "play_next", expectedRevision: 8, idempotencyKey: expect.any(String), mediaIds: ["episode-5"] },
        expect.any(AbortSignal),
      ),
    );
  });

  it("keeps repeat mode authoritative and recovers revision conflicts without replaying the mutation", async () => {
    const source = new FixturePorticoDataSource();
    const initialQueue = queue();
    const refreshedQueue = queue({ repeatMode: "one", revision: 9 });
    const repeatedQueue = queue({ repeatMode: "off", revision: 10 });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback(),
    );
    const readQueue = vi
      .spyOn(source as PorticoDataSource, "playbackSessionQueue")
      .mockResolvedValueOnce(initialQueue)
      .mockResolvedValueOnce(refreshedQueue);
    const mutate = vi
      .spyOn(source as PorticoDataSource, "mutatePlaybackSessionQueue")
      .mockRejectedValueOnce(
        new ApiError(409, "queue_revision_conflict", "Queue changed."),
      )
      .mockResolvedValueOnce(repeatedQueue);

    renderPlayer(source);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByLabelText("Queue"));
    const queueDialog = await screen.findByRole("dialog", { name: "Queue" });

    fireEvent.click(
      within(queueDialog).getByRole("button", { name: "Repeat off" }),
    );
    await waitFor(() =>
      expect(
        within(queueDialog).getByRole("button", { name: "Repeat one" }),
      ).toBeInTheDocument(),
    );
    expect(within(queueDialog).getByRole("alert")).toHaveTextContent(
      "Queue changed on another device. Review the latest queue and try again.",
    );
    expect(readQueue).toHaveBeenCalledTimes(2);
    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate).toHaveBeenLastCalledWith(
      "session-1",
      { action: "set_repeat", expectedRevision: 7, idempotencyKey: expect.any(String), repeatMode: "all" },
      expect.any(AbortSignal),
    );

    fireEvent.click(
      within(queueDialog).getByRole("button", { name: "Repeat one" }),
    );
    await waitFor(() =>
      expect(
        within(queueDialog).getByRole("button", { name: "Repeat off" }),
      ).toBeInTheDocument(),
    );
    expect(mutate).toHaveBeenCalledTimes(2);
    expect(mutate).toHaveBeenLastCalledWith(
      "session-1",
      { action: "set_repeat", expectedRevision: 9, idempotencyKey: expect.any(String), repeatMode: "off" },
      expect.any(AbortSignal),
    );
  });

  it("retains terminal authority while Up Next prepares and atomically hands off on demand", async () => {
    const source = new FixturePorticoDataSource();
    const current = playback();
    const preparedPlayback = playback({
      sessionId: "session-prepared",
      media: coreMedia("episode-2", "Palindrome"),
      currentQueueEntryId: "entry-episode-2",
      queue: [queueEntry("episode-3", "The Myth of Sisyphus")],
    });
    const handedOff = playback({
      sessionId: "session-2",
      media: coreMedia("episode-2", "Palindrome"),
      currentQueueEntryId: "entry-episode-2",
      queue: [queueEntry("episode-3", "The Myth of Sisyphus")],
    });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      current,
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());
    const prepare = vi
      .spyOn(source as PorticoDataSource, "prepareNextPlayback")
      .mockResolvedValue({
        preparedSessionId: "prepared-2",
        playback: preparedPlayback,
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
        preloadPolicy: "metadata",
        handoffMode: "replace",
        queue: preparedPlayback.queue,
        queueRevision: preparedPlayback.queueRevision,
        playbackRevision: preparedPlayback.playbackRevision,
      });
    const handoff = vi
      .spyOn(source as PorticoDataSource, "handoffPlayback")
      .mockResolvedValue(handedOff);
    const touch = vi.spyOn(source as PorticoDataSource, "touchPlayback");
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);

    renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    await waitFor(() =>
      expect(HTMLMediaElement.prototype.play).toHaveBeenCalled(),
    );
    const endedMedia = surface.querySelector("video") as HTMLVideoElement;
    fireEvent.ended(endedMedia);
    for (let index = 0; index < 8; index += 1) fireEvent.ended(endedMedia);

    const upNext = await screen.findByRole("region", { name: "Up next" });
    expect(within(upNext).getByText("Palindrome")).toBeInTheDocument();
    expect(
      within(upNext).getByText(/Playing in \d+ seconds?/),
    ).toBeInTheDocument();
    expect(prepare).toHaveBeenCalledWith(
      "session-1",
      expect.any(AbortSignal),
      expect.objectContaining({ intent: expect.any(Object) }),
    );
    expect(prepare).toHaveBeenCalledTimes(1);
    expect(stop).not.toHaveBeenCalled();
    const touchesAfterCompletion = touch.mock.calls.length;
    fireEvent.pause(endedMedia);
    fireEvent.timeUpdate(endedMedia);
    expect(touch).toHaveBeenCalledTimes(touchesAfterCompletion);

    fireEvent.click(within(upNext).getByRole("button", { name: "Play now" }));
    await waitFor(() =>
      expect(handoff).toHaveBeenCalledWith(
        "session-1",
        expect.objectContaining({
          preparedSessionId: "prepared-2",
          requestId: expect.stringMatching(/^web-[0-9a-f-]{36}$/),
          previousTerminal: { disposition: "completed", positionSeconds: 120, durationSeconds: 120 },
          intent: expect.any(Object),
        }),
        expect.any(AbortSignal),
      ),
    );
    expect(
      await screen.findByLabelText("Now playing Palindrome"),
    ).toBeInTheDocument();
    expect(stop).not.toHaveBeenCalled();
  });

  it("closes an ended source exactly once when Up Next is cancelled", async () => {
    const source = new FixturePorticoDataSource();
    const current = playback();
    const preparedPlayback = playback({
      sessionId: "session-prepared",
      media: coreMedia("episode-2", "Palindrome"),
      currentQueueEntryId: "entry-episode-2",
    });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(current);
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    vi.spyOn(source as PorticoDataSource, "prepareNextPlayback").mockResolvedValue({
      preparedSessionId: "prepared-2",
      playback: preparedPlayback,
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    } as never);
    const handoff = vi.spyOn(source as PorticoDataSource, "handoffPlayback");
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);

    renderPlayer(source);
    const media = (await screen.findByLabelText("Now playing The Castle")).querySelector("video") as HTMLVideoElement;
    fireEvent.ended(media);
    const upNext = await screen.findByRole("region", { name: "Up next" });
    expect(stop).not.toHaveBeenCalled();
    fireEvent.click(within(upNext).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(stop).toHaveBeenCalledWith(
      "session-1",
      { disposition: "completed", positionSeconds: 120, durationSeconds: 120 },
      expect.any(AbortSignal),
      true,
    ));
    fireEvent.ended(media);
    expect(stop).toHaveBeenCalledTimes(1);
    expect(handoff).not.toHaveBeenCalled();
  });

  it("exact-retries an ambiguous natural handoff with the same immutable request", async () => {
    const source = new FixturePorticoDataSource();
    const currentMedia = { ...coreMedia("track-1", "Signal One"), entityKind: "track" as const };
    const nextMedia = { ...coreMedia("track-2", "Signal Two"), entityKind: "track" as const };
    const current = playback({ media: currentMedia, currentQueueEntryId: "entry-track-1", queue: [{ entryId: "entry-track-2", media: nextMedia }] });
    const replacement = playback({ sessionId: "session-2", media: nextMedia, currentQueueEntryId: "entry-track-2", queue: [] });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(current);
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue({ current: { entryId: "entry-track-1", media: currentMedia }, items: [{ entryId: "entry-track-2", media: nextMedia }], total: 1 }));
    vi.spyOn(source as PorticoDataSource, "prepareNextPlayback").mockResolvedValue({
      preparedSessionId: "prepared-track-2",
      playback: replacement,
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    } as never);
    const handoff = vi.spyOn(source as PorticoDataSource, "handoffPlayback")
      .mockRejectedValueOnce(new TypeError("response lost"))
      .mockResolvedValueOnce(replacement);
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);

    renderPlayer(source, "track-1");
    const media = (await screen.findByLabelText("Now playing Signal One")).querySelector("video") as HTMLVideoElement;
    fireEvent.ended(media);
    await waitFor(() => expect(handoff).toHaveBeenCalledTimes(2));
    expect(handoff.mock.calls[1][1]).toBe(handoff.mock.calls[0][1]);
    expect(handoff.mock.calls[0][1]).toEqual(expect.objectContaining({
      requestId: expect.stringMatching(/^web-[0-9a-f-]{36}$/),
      previousTerminal: { disposition: "completed", positionSeconds: 120, durationSeconds: 120 },
    }));
    expect(await screen.findByLabelText("Now playing Signal Two")).toBeInTheDocument();
    expect(stop).not.toHaveBeenCalled();
  });

  it("uses a stopped terminal snapshot for explicit Previous", async () => {
    const source = new FixturePorticoDataSource();
    const replacement = playback({ sessionId: "session-previous", media: coreMedia("episode-0", "Waiting for Dutch"), currentQueueEntryId: "entry-episode-0" });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    const handoff = vi.spyOn(source as PorticoDataSource, "handoffPlayback").mockResolvedValue(replacement);
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);
    renderPlayer(source);
    const media = (await screen.findByLabelText("Now playing The Castle")).querySelector("video") as HTMLVideoElement;
    media.currentTime = 37.5;
    fireEvent.click(screen.getByRole("button", { name: "Previous item" }));
    await waitFor(() => expect(handoff).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({
        entryId: "entry-episode-0",
        startSeconds: 0,
        previousTerminal: { disposition: "stopped", positionSeconds: 37.5, durationSeconds: 120 },
      }),
      expect.any(AbortSignal),
    ));
    expect(stop).not.toHaveBeenCalled();
  });

  it("uses a stopped terminal snapshot for explicit Next", async () => {
    const source = new FixturePorticoDataSource();
    const nextMedia = coreMedia("episode-2", "Palindrome");
    const replacement = playback({ sessionId: "session-next", media: nextMedia, currentQueueEntryId: "entry-episode-2" });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    vi.spyOn(source as PorticoDataSource, "prepareNextPlayback").mockResolvedValue({
      preparedSessionId: "prepared-next",
      playback: replacement,
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    } as never);
    const handoff = vi.spyOn(source as PorticoDataSource, "handoffPlayback").mockResolvedValue(replacement);
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);
    renderPlayer(source);
    const media = (await screen.findByLabelText("Now playing The Castle")).querySelector("video") as HTMLVideoElement;
    media.currentTime = 48.25;
    fireEvent.click(screen.getByRole("button", { name: "Next item" }));
    await waitFor(() => expect(handoff).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({
        preparedSessionId: "prepared-next",
        entryId: "entry-episode-2",
        previousTerminal: { disposition: "stopped", positionSeconds: 48.25, durationSeconds: 120 },
      }),
      expect.any(AbortSignal),
    ));
    expect(stop).not.toHaveBeenCalled();
  });

  it("keeps the old session authoritative when a deliberate handoff is definitively rejected", async () => {
    const source = new FixturePorticoDataSource();
    const nextMedia = coreMedia("episode-2", "Palindrome");
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue());
    vi.spyOn(source as PorticoDataSource, "prepareNextPlayback").mockResolvedValue({
      preparedSessionId: "prepared-next",
      playback: playback({ sessionId: "prepared-next-session", media: nextMedia, currentQueueEntryId: "entry-episode-2" }),
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    } as never);
    const handoff = vi.spyOn(source as PorticoDataSource, "handoffPlayback")
      .mockRejectedValue(new ApiError(409, "stale_event", "The source remains active."));
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);
    const touch = vi.spyOn(source as PorticoDataSource, "touchPlayback");
    renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    const media = surface.querySelector("video") as HTMLVideoElement;
    media.currentTime = 48.25;

    fireEvent.click(screen.getByRole("button", { name: "Next item" }));
    await waitFor(() => expect(handoff).toHaveBeenCalledTimes(1));
    await screen.findByRole("alert");
    expect(screen.getByLabelText("Now playing The Castle")).toBeInTheDocument();
    expect(stop).not.toHaveBeenCalled();
    const touchesBeforeResume = touch.mock.calls.length;
    fireEvent.pause(media);
    await waitFor(() => expect(touch.mock.calls.length).toBeGreaterThan(touchesBeforeResume));
  });

  it("atomically wraps repeat-all with a completed terminal and explicit restart", async () => {
    const source = new FixturePorticoDataSource();
    const repeatQueue = queue({ items: [], total: 0, repeatMode: "all", history: [historyEntry("episode-0", "Waiting for Dutch")] });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(playback({ queue: [], repeatMode: "all" }));
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(repeatQueue);
    const handoff = vi.spyOn(source as PorticoDataSource, "handoffPlayback").mockResolvedValue(playback({ sessionId: "session-wrap" }));
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);
    renderPlayer(source);
    const media = (await screen.findByLabelText("Now playing The Castle")).querySelector("video") as HTMLVideoElement;
    fireEvent.ended(media);
    await waitFor(() => expect(handoff).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({
        entryId: "entry-episode-0",
        startSeconds: 0,
        previousTerminal: { disposition: "completed", positionSeconds: 120, durationSeconds: 120 },
      }),
      expect.any(AbortSignal),
    ));
    expect(stop).not.toHaveBeenCalled();
  });

  it("shows queue exhaustion and replays with a fresh start after the source is closed", async () => {
    const source = new FixturePorticoDataSource();
    const current = playback({ queue: [] });
    const emptyQueue = { ...queue(), items: [], total: 0 };
    const start = vi.spyOn(source as PorticoDataSource, "startPlayback")
      .mockResolvedValueOnce(current)
      .mockResolvedValueOnce(playback({ sessionId: "session-replay", queue: [] }));
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(emptyQueue);
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);
    const handoff = vi.spyOn(source as PorticoDataSource, "handoffPlayback");

    renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    await waitFor(() =>
      expect(HTMLMediaElement.prototype.play).toHaveBeenCalled(),
    );
    fireEvent.ended(surface.querySelector("video") as HTMLVideoElement);

    const complete = await screen.findByRole("region", {
      name: "Playback is complete.",
    });
    expect(
      within(complete).getByText("You're all caught up"),
    ).toBeInTheDocument();
    await waitFor(() => expect(stop).toHaveBeenCalledWith(
      "session-1",
      { disposition: "completed", positionSeconds: 120, durationSeconds: 120 },
      expect.any(AbortSignal),
      true,
    ));
    fireEvent.click(within(complete).getByRole("button", { name: "Replay" }));
    await waitFor(() =>
      expect(start).toHaveBeenLastCalledWith(
        "episode-1",
        expect.objectContaining({
          startSeconds: 0,
        }),
        expect.any(AbortSignal),
      ),
    );
    expect(handoff).not.toHaveBeenCalled();
  });

  it("shares one atomic terminal owner across natural end, completed UI close, and repeated cleanup events", async () => {
    const source = new FixturePorticoDataSource();
    const current = playback({ queue: [] });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(current);
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue({ items: [], total: 0 }));
    const touch = vi.spyOn(source as PorticoDataSource, "touchPlayback");
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockImplementation(() => new Promise<void>(() => undefined));

    renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    const media = surface.querySelector("video") as HTMLVideoElement;
    Object.defineProperty(media, "duration", { configurable: true, value: 120 });
    media.currentTime = 120;

    fireEvent.ended(media);
    fireEvent.ended(media);
    await screen.findByRole("region", { name: "Playback is complete." });
    await waitFor(() => expect(stop).toHaveBeenCalledTimes(1));
    expect(stop).toHaveBeenCalledWith(
      "session-1",
      { disposition: "completed", positionSeconds: 120, durationSeconds: 120 },
      expect.any(AbortSignal),
      true,
    );
    const touchesAtFence = touch.mock.calls.length;

    fireEvent.pause(media);
    fireEvent.timeUpdate(media);
    fireEvent.ended(media);
    fireEvent.click(screen.getAllByRole("button", { name: "Close player" })[0]);

    expect(stop).toHaveBeenCalledTimes(1);
    expect(touch).toHaveBeenCalledTimes(touchesAtFence);
    expect(screen.queryByLabelText("Now playing The Castle")).not.toBeInTheDocument();
  });

  it("hands off the canonical next queue item before terminalizing an ended music session", async () => {
    const transitionTelemetry = vi.spyOn(console, "info").mockImplementation(() => undefined);
    const source = new FixturePorticoDataSource();
    const currentMedia = { ...coreMedia("track-1", "Signal One"), entityKind: "track" as const };
    const nextMedia = { ...coreMedia("track-2", "Signal Two"), entityKind: "track" as const };
    const thirdMedia = { ...coreMedia("track-3", "Signal Three"), entityKind: "track" as const };
    const current = playback({ media: currentMedia, currentQueueEntryId: "entry-track-1", queue: [{ entryId: "entry-track-2", media: nextMedia }, { entryId: "entry-track-3", media: thirdMedia }] });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(current);
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue({ current: { entryId: "entry-track-1", media: currentMedia }, items: [{ entryId: "entry-track-2", media: nextMedia }, { entryId: "entry-track-3", media: thirdMedia }], total: 2 }));
    const prepare = vi.spyOn(source as PorticoDataSource, "prepareNextPlayback").mockResolvedValue({ preparedSessionId: "prepared-track-2", playback: playback({ sessionId: "prepared", media: nextMedia, currentQueueEntryId: "entry-track-2", queue: [{ entryId: "entry-track-3", media: thirdMedia }] }), expiresAt: new Date(Date.now() + 60_000).toISOString() } as never);
    let resolveHandoff!: (value: PlaybackResponse) => void;
    const handoff = vi.spyOn(source as PorticoDataSource, "handoffPlayback").mockImplementation(() => new Promise<PlaybackResponse>((resolve) => { resolveHandoff = resolve; }));
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);
    renderPlayer(source, "track-1");
    const media = (await screen.findByLabelText("Now playing Signal One")).querySelector("video") as HTMLVideoElement;
    fireEvent.ended(media);
    await waitFor(() => expect(prepare).toHaveBeenCalledWith("session-1", expect.any(AbortSignal), expect.objectContaining({ entryId: "entry-track-2" })));
    await waitFor(() => expect(handoff).toHaveBeenCalledTimes(1));
    expect(prepare).toHaveBeenCalledTimes(1);
    expect(handoff).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({
        preparedSessionId: "prepared-track-2",
        requestId: expect.stringMatching(/^web-[0-9a-f-]{36}$/),
        previousTerminal: { disposition: "completed", positionSeconds: 120, durationSeconds: 120 },
      }),
      expect.any(AbortSignal),
    );
    expect(transitionTelemetry).toHaveBeenCalledWith("[portico-playback-handoff]", expect.stringMatching(/"phase":"request".*"sourceSessionId":"session-1".*"preparedSessionId":"prepared-track-2".*"requestId":"web-[0-9a-f-]{36}"/));
    expect(stop).not.toHaveBeenCalled();
    resolveHandoff(playback({ sessionId: "session-2", media: nextMedia, queue: [queueEntry(thirdMedia.id, thirdMedia.title)] }));
    await waitFor(() => expect(screen.getByLabelText("Now playing Signal Two")).toBeInTheDocument());
    expect(stop).not.toHaveBeenCalled();
  });

  it("terminalizes an ended music session once when its prepared handoff fails", async () => {
    const source = new FixturePorticoDataSource();
    const currentMedia = { ...coreMedia("track-1", "Signal One"), entityKind: "track" as const };
    const nextMedia = { ...coreMedia("track-2", "Signal Two"), entityKind: "track" as const };
    const current = playback({ media: currentMedia, currentQueueEntryId: "entry-track-1", queue: [{ entryId: "entry-track-2", media: nextMedia }] });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(current);
    vi.spyOn(source as PorticoDataSource, "playbackSessionQueue").mockResolvedValue(queue({ current: { entryId: "entry-track-1", media: currentMedia }, items: [{ entryId: "entry-track-2", media: nextMedia }], total: 1 }));
    vi.spyOn(source as PorticoDataSource, "prepareNextPlayback").mockResolvedValue({ preparedSessionId: "prepared-track-2", playback: playback({ sessionId: "prepared", media: nextMedia, currentQueueEntryId: "entry-track-2" }), expiresAt: new Date(Date.now() + 60_000).toISOString() } as never);
    const handoff = vi.spyOn(source as PorticoDataSource, "handoffPlayback").mockRejectedValue(new ApiError(409, "prepared_session_expired", "handoff failed"));
    const touch = vi.spyOn(source as PorticoDataSource, "touchPlayback");
    const stop = vi.spyOn(source as PorticoDataSource, "stopPlayback").mockResolvedValue(undefined);
    renderPlayer(source, "track-1");
    const media = (await screen.findByLabelText("Now playing Signal One")).querySelector("video") as HTMLVideoElement;
    fireEvent.ended(media);
    await waitFor(() => expect(stop).toHaveBeenCalledTimes(1));
    expect(handoff).toHaveBeenCalledTimes(1);
    expect(stop).toHaveBeenCalledWith(
      "session-1",
      { disposition: "completed", positionSeconds: 120, durationSeconds: 120 },
      expect.any(AbortSignal),
      true,
    );
    const touches = touch.mock.calls.length;
    fireEvent.timeUpdate(media);
    fireEvent.ended(media);
    expect(stop).toHaveBeenCalledTimes(1);
    expect(touch).toHaveBeenCalledTimes(touches);
  });

  it("closes an established playback session with a direct interruption message after an unrecoverable media failure", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback(),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());

    renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    const media = surface.querySelector("video") as HTMLVideoElement;
    await waitFor(() =>
      expect(HTMLMediaElement.prototype.play).toHaveBeenCalled(),
    );
    fireEvent.waiting(media);
    expect(await screen.findByText("Buffering")).toBeInTheDocument();

    vi.mocked(HTMLMediaElement.prototype.play).mockImplementation(
      function stalledPlay(this: HTMLMediaElement) {
        Object.defineProperty(this, "paused", {
          configurable: true,
          value: false,
        });
        this.dispatchEvent(new Event("play"));
        return Promise.resolve();
      },
    );

    Object.defineProperty(media, "error", {
      configurable: true,
      value: { code: 2 },
    });
    vi.useFakeTimers();
    // The first network failure is reserved for a silent route/grant/source
    // rebase before the normal media-element retry budget is exhausted.
    fireEvent.error(media);
    await vi.advanceTimersByTimeAsync(0);
    for (const delay of [0, 400, 1_200]) {
      fireEvent.error(media);
      await vi.advanceTimersByTimeAsync(delay);
    }
    fireEvent.error(media);
    await vi.advanceTimersByTimeAsync(0);
    vi.useRealTimers();
    expect(await screen.findByText("Playback stopped")).toBeInTheDocument();
    expect(screen.getByText(/several reconnect attempts/i)).toBeInTheDocument();
    expect(
      screen.queryByLabelText("Now playing The Castle"),
    ).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Dismiss playback message" }),
    );
    expect(screen.queryByText("Playback stopped")).not.toBeInTheDocument();
  });

  it("uses a live state instead of exposing VOD seeking controls", async () => {
    const source = new FixturePorticoDataSource();
    const liveMedia = {
      ...coreMedia("channel-1", "News 7"),
      entityKind: "live-channel",
    } as PlaybackResponse["media"];
    const live = playback({
      media: liveMedia,
      isLive: true,
      queue: [],
      timeline: { type: "live", canPause: false, canSeek: false },
    });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      live,
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue({
      ...queue(),
      current: { entryId: "entry-channel-1", media: liveMedia },
      items: [],
      history: [],
      total: 0,
    });

    renderPlayer(source, "channel-1");
    const surface = await screen.findByLabelText("Now playing News 7");
    expect(surface).toHaveClass("mode-live");
    const channelLogo = surface.querySelector(".player-copy-logo");
    expect(channelLogo).toHaveAttribute(
      "src",
      "http://localhost:3000/thumb.jpg",
    );
    fireEvent.error(channelLogo as HTMLImageElement);
    await waitFor(() =>
      expect(
        surface.querySelector(".player-copy-art-fallback"),
      ).toBeInTheDocument(),
    );
    expect(within(surface).getByText("Live")).toBeInTheDocument();
    expect(
      within(surface).queryByLabelText("Playback position"),
    ).not.toBeInTheDocument();
    expect(
      within(surface).getByRole("button", { name: "Rewind 10 seconds" }),
    ).toBeDisabled();
    expect(
      within(surface).getByRole("button", { name: "Forward 30 seconds" }),
    ).toBeDisabled();
    expect(
      within(surface).getByRole("button", { name: "Next item" }),
    ).toBeDisabled();
  });

  it("resolves playback artwork against the selected server and scopes API images to the media grant", async () => {
    const source = new FixturePorticoDataSource();
    const media = {
      ...coreMedia("episode-1", "The Castle"),
      images: {
        poster: "/api/artwork/episode-1/poster",
        backdrop: "/api/artwork/episode-1/backdrop",
        thumb: "/api/artwork/episode-1/thumb",
      },
    } as PlaybackResponse["media"];
    vi.spyOn(
      source as PorticoDataSource,
      "playbackResourceUrl",
    ).mockImplementation((path) => `https://server.example${path}`);
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback({ media }),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue({ current: { entryId: `entry-${media.id}`, media } }));

    renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    expect(surface.querySelector(".player-copy-art")).toHaveAttribute(
      "src",
      "https://server.example/api/artwork/episode-1/poster",
    );
    expect(surface.querySelector("video")).toHaveAttribute(
      "poster",
      "https://server.example/api/artwork/episode-1/backdrop",
    );
  });

  it("presents subtitle, lyric, and audio capabilities in their contract locations", async () => {
    const videoSource = new FixturePorticoDataSource();
    const subtitleStream = {
      id: "subtitle-en",
      kind: "subtitle",
      codec: "srt",
      language: "en",
      displayTitle: "English",
    } as PlaybackResponse["subtitleStreams"][number];
    vi.spyOn(
      videoSource as PorticoDataSource,
      "startPlayback",
    ).mockResolvedValue(playback({ subtitleStreams: [subtitleStream] }));
    vi.spyOn(
      videoSource as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());

    const videoView = renderPlayer(videoSource);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Subtitles" }));
    const subtitleDialog = await screen.findByRole("dialog", {
      name: "Subtitles",
    });
    const subtitleChoices = within(subtitleDialog).getByRole("radiogroup", {
      name: "Subtitle track",
    });
    expect(
      within(subtitleChoices).getByRole("radio", { name: "Off" }),
    ).toHaveAttribute("aria-checked", "true");
    expect(
      within(subtitleChoices).getByRole("radio", { name: /English/ }),
    ).toHaveAttribute("aria-checked", "false");
    videoView.unmount();

    const audioSource = new FixturePorticoDataSource();
    const musicMedia = {
      ...coreMedia("track-lyrics", "Night Drive"),
      entityKind: "track",
      lyrics: [
        {
          id: "lyrics-1",
          source: "embedded",
          format: "lrc",
          synced: true,
          text: "[00:00.00]Under city lights\n[00:12.00]The night is ours",
          createdAt: "2026-01-01T00:00:00Z",
        },
      ],
    } as PlaybackResponse["media"];
    vi.spyOn(
      audioSource as PorticoDataSource,
      "startPlayback",
    ).mockResolvedValue(playback({ media: musicMedia, queue: [] }));
    vi.spyOn(
      audioSource as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue({
      ...queue(),
      current: { entryId: "entry-track-lyrics", media: musicMedia },
      items: [],
      history: [],
      total: 0,
    });

    renderPlayer(audioSource, "track-lyrics");
    const audioSurface = await screen.findByLabelText(
      "Now playing Night Drive",
    );
    const audioElement = audioSurface.querySelector(
      "video",
    ) as HTMLVideoElement;
    audioElement.currentTime = 13;
    fireEvent.timeUpdate(audioElement);

    fireEvent.click(screen.getByRole("button", { name: "Lyrics" }));
    const lyricDialog = await screen.findByRole("dialog", { name: "Lyrics" });
    const synchronizedLyrics = within(lyricDialog).getByRole("region", {
      name: "Synchronized lyrics",
    });
    expect(
      synchronizedLyrics.querySelector('[aria-current="true"]'),
    ).toHaveTextContent("The night is ours");
    fireEvent.wheel(synchronizedLyrics);
    expect(
      within(lyricDialog).getByRole("button", { name: "Follow current lyric" }),
    ).toBeInTheDocument();
    fireEvent.click(
      within(lyricDialog).getByRole("button", { name: "Follow current lyric" }),
    );
    expect(
      within(lyricDialog).queryByRole("button", {
        name: "Follow current lyric",
      }),
    ).not.toBeInTheDocument();
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Lyrics" }),
      ).not.toBeInTheDocument(),
    );

    fireEvent.click(screen.getByRole("button", { name: "Playback settings" }));
    const settingsDialog = await screen.findByRole("dialog", {
      name: "Playback settings",
    });
    fireEvent.click(
      within(settingsDialog).getByRole("combobox", { name: "Audio" }),
    );
    expect(
      within(settingsDialog).getByRole("option", { name: /English/i }),
    ).toHaveAttribute("aria-selected", "true");
    expect(
      screen.queryByRole("button", { name: "Subtitles" }),
    ).not.toBeInTheDocument();
  });

  it("uses one server-issued quality selection and adopts the returned sealed stream", async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue(
      "Mozilla/5.0 Safari/605.1.15",
    );
    let selectedPlayback = playback({
      qualityOffers: {
        ...playback().qualityOffers,
        offerRevision: "qrev-quality-choice",
        offers: [
          { selectionId: "qsel-automatic", label: "Automatic", kind: "automatic" },
          { selectionId: "qsel-original", label: "Original Quality", kind: "original" },
          { selectionId: "qsel-720p", label: "720p", kind: "fixed", maxVideoBitrateBps: 4_000_000, targetDisplayHeight: 720 },
        ],
      },
      audioStreams: [
        {
          id: "audio-1",
          kind: "audio",
          codec: "aac",
          language: "en",
          displayTitle: "English",
        },
        {
          id: "audio-2",
          kind: "audio",
          codec: "aac",
          language: "fr",
          displayTitle: "French",
        },
      ],
    });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      selectedPlayback,
    );
    const renegotiate = vi
      .spyOn(source as PorticoDataSource, "renegotiatePlayback")
      .mockImplementation(async (_sessionId, request) => {
        const nextSelection = request.quality ?? selectedPlayback.qualitySelection;
        const nextQuality = nextSelection.mode === "explicit" ? nextSelection.selectionId : "automatic";
        const nextAudio = request.audioStreamId ?? selectedPlayback.selectedAudioStreamId ?? "audio-1";
        const nextSource = nextQuality === "qsel-720p"
          ? nextAudio === "audio-2"
            ? "/api/playback-resources/hls-720p-fr?signature=server-owned"
            : "/api/playback-resources/hls-720p-en?signature=server-owned"
          : "/api/media/episode-1/stream.mp4";
        selectedPlayback = {
          ...selectedPlayback,
          playbackRevision: selectedPlayback.playbackRevision + 1,
          sourceUrl: nextSource,
          streamFormat: nextQuality === "qsel-720p" ? "hls" : "direct",
          qualitySelection: nextSelection,
          selectedAudioStreamId: nextAudio,
          resources: [{
            id: `active-${nextQuality}-${nextAudio}`,
            sourceUrl: nextSource,
            streamFormat: nextQuality === "qsel-720p" ? "hls" : "direct",
            audioStreamId: nextAudio,
            subtitleMode: "off",
            default: true,
          }],
          mediaGrant: {
            token: `grant-${selectedPlayback.playbackRevision + 1}`,
            expiresAt: "2099-01-01T00:00:00Z",
          },
        };
        return selectedPlayback;
      });
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());

    renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Playback settings" }));
    const settings = await screen.findByRole("dialog", {
      name: "Playback settings",
    });
    fireEvent.click(
      within(settings).getByRole("combobox", { name: "Quality" }),
    );
    fireEvent.click(within(settings).getByRole("option", { name: /720p/i }));
    await waitFor(() =>
      expect((surface.querySelector("video") as HTMLVideoElement).src).toBe(
        "http://localhost:3000/api/playback-resources/hls-720p-en?signature=server-owned",
      ),
    );
    fireEvent.click(within(settings).getByRole("combobox", { name: "Audio" }));
    fireEvent.click(within(settings).getByRole("option", { name: /French/i }));
    await waitFor(() =>
      expect((surface.querySelector("video") as HTMLVideoElement).src).toBe(
        "http://localhost:3000/api/playback-resources/hls-720p-fr?signature=server-owned",
      ),
    );
    expect(renegotiate).toHaveBeenNthCalledWith(
      1,
      "session-1",
      expect.objectContaining({ quality: { mode: "explicit", selectionId: "qsel-720p", qualityOfferRevision: "qrev-quality-choice" }, expectedRevision: 1 }),
      expect.any(AbortSignal),
    );
    expect(renegotiate).toHaveBeenNthCalledWith(
      2,
      "session-1",
      expect.objectContaining({
        audioStreamId: "audio-2",
        expectedRevision: 2,
      }),
      expect.any(AbortSignal),
    );
  });

  it("supports keyboard traversal and restores focus after choosing a player setting", async () => {
    const source = new FixturePorticoDataSource();
    const initialPlayback = playback({
      qualityOffers: {
        ...playback().qualityOffers,
        offerRevision: "qrev-keyboard",
        offers: [
          { selectionId: "qsel-automatic", label: "Automatic", kind: "automatic" },
          { selectionId: "qsel-original", label: "Original Quality", kind: "original" },
          { selectionId: "qsel-720p", label: "720p · 4 Mbps", kind: "fixed", maxVideoBitrateBps: 4_000_000, targetDisplayHeight: 720 },
          { selectionId: "qsel-480p", label: "480p · 1.5 Mbps", kind: "fixed", maxVideoBitrateBps: 1_500_000, targetDisplayHeight: 480 },
        ],
      },
    });
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      initialPlayback,
    );
    vi.spyOn(
      source as PorticoDataSource,
      "renegotiatePlayback",
    ).mockImplementation(async (_sessionId, request) => ({
      ...initialPlayback,
      playbackRevision: initialPlayback.playbackRevision + 1,
      qualitySelection: request.quality ?? initialPlayback.qualitySelection,
      mediaGrant: { token: "grant-quality", expiresAt: "2099-01-01T00:00:00Z" },
    }));
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());

    renderPlayer(source);
    await screen.findByLabelText("Now playing The Castle");
    fireEvent.click(screen.getByRole("button", { name: "Playback settings" }));
    const settings = await screen.findByRole("dialog", {
      name: "Playback settings",
    });
    const quality = within(settings).getByRole("combobox", { name: "Quality" });

    quality.focus();
    fireEvent.keyDown(quality, { key: "ArrowDown" });
    expect(within(settings).getByRole("option", { name: /Original Quality/ })).toBeInTheDocument();
    expect(within(settings).getByRole("option", { name: /720p · 4 Mbps/ })).toBeInTheDocument();
    const automatic = within(settings).getByRole("option", {
      name: /Automatic/i,
    });
    await waitFor(() => expect(automatic).toHaveFocus());
    fireEvent.keyDown(automatic, { key: "ArrowDown" });
    const original = within(settings).getByRole("option", { name: /Original/i });
    expect(original).toHaveFocus();
    fireEvent.keyDown(original, { key: "ArrowDown" });
    const medium = within(settings).getByRole("option", { name: /720p/i });
    expect(medium).toHaveFocus();
    fireEvent.keyDown(medium, { key: "End" });
    const low = within(settings).getByRole("option", { name: /480p/i });
    expect(low).toHaveFocus();
    fireEvent.click(low);
    await waitFor(() => expect(quality).toHaveFocus());
    expect(quality).toHaveTextContent("480p");

    fireEvent.keyDown(quality, { key: "ArrowUp" });
    await waitFor(() => expect(medium).toHaveFocus());
    fireEvent.keyDown(medium, { key: "Escape" });
    expect(quality).toHaveFocus();
    expect(quality).toHaveAttribute("aria-expanded", "false");
  });

  it("applies playback speed, persists volume, and exposes contract chapters and skip prompts", async () => {
    const source = new FixturePorticoDataSource();
    const media = {
      ...coreMedia("episode-1", "The Castle"),
      segments: [
        {
          id: "intro-1",
          type: "intro",
          startSeconds: 0,
          endSeconds: 18,
          automaticSafe: false,
        },
      ],
    } as PlaybackResponse["media"];
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback({
        media,
        chapters: [
          {
            id: "chapter-1",
            title: "Opening",
            startSeconds: 0,
            endSeconds: 30,
            thumbUrl: "/api/artwork/episode-1/chapter-1.jpg",
          },
          {
            id: "chapter-2",
            title: "The plan",
            startSeconds: 30,
            endSeconds: 65,
          },
        ],
      }),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue({ current: { entryId: `entry-${media.id}`, media } }));

    renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    const mediaElement = surface.querySelector("video") as HTMLVideoElement;
    expect(
      screen.getByRole("button", { name: "Skip Intro" }),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Dismiss skip intro prompt" }),
    );
    expect(
      screen.queryByRole("button", { name: "Skip Intro" }),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Chapters" }));
    const chapters = await screen.findByRole("dialog", { name: "Chapters" });
    expect(within(chapters).getByText("Opening")).toBeInTheDocument();
    expect(chapters.querySelector("img")?.getAttribute("src")).toContain(
      "/api/artwork/episode-1/chapter-1.jpg",
    );
    expect(chapters.querySelector("img")?.getAttribute("src")).not.toContain(
      "grant=",
    );
    fireEvent.click(within(chapters).getByText("The plan"));
    expect(mediaElement.currentTime).toBe(30);
    fireEvent.keyDown(window, { key: "Escape" });

    fireEvent.click(screen.getByRole("button", { name: "Playback settings" }));
    const settings = await screen.findByRole("dialog", {
      name: "Playback settings",
    });
    fireEvent.click(
      within(settings).getByRole("combobox", { name: "Playback speed" }),
    );
    fireEvent.click(within(settings).getByRole("option", { name: "1.5×" }));
    expect(mediaElement.playbackRate).toBe(1.5);
    fireEvent.keyDown(window, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Volume" }));
    const volume = await screen.findByRole("dialog", { name: "Volume" });
    fireEvent.change(within(volume).getByLabelText("Volume"), {
      target: { value: "0.4" },
    });
    expect(
      JSON.parse(localStorage.getItem("portico.player.volume.v1") ?? "{}"),
    ).toMatchObject({ volume: 0.4, muted: false });
  });

  it("shows real trickplay tiles and keeps diagnostics behind the saved preference", async () => {
    const source = new FixturePorticoDataSource();
    localStorage.setItem(
      "portico.web.installation-preferences.v1",
      JSON.stringify({ playbackDiagnostics: true }),
    );
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback(),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());
    const mediaTrickplay = vi
      .spyOn(source as PorticoDataSource, "mediaTrickplay")
      .mockResolvedValue([
        {
          id: "trick-1",
          mediaId: "episode-1",
          width: 160,
          height: 90,
          tileWidth: 160,
          tileHeight: 90,
          intervalSeconds: 10,
          durationSeconds: 120,
          tileCount: 12,
          stale: false,
          createdAt: "2026-01-01T00:00:00Z",
        },
      ]);

    renderPlayer(source);
    const playerSurface = await screen.findByLabelText(
      "Now playing The Castle",
    );
    await waitFor(() => expect(mediaTrickplay).toHaveBeenCalled());
    const timeline = screen.getByLabelText("Playback position");
    vi.spyOn(timeline, "getBoundingClientRect").mockReturnValue({
      left: 0,
      right: 100,
      width: 100,
      top: 0,
      bottom: 10,
      height: 10,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });
    await waitFor(() => {
      fireEvent.mouseMove(timeline, { clientX: 50 });
      expect(
        document.querySelector(".trickplay-preview img"),
      ).toBeInTheDocument();
    });
    const preview = document.querySelector(
      ".trickplay-preview img",
    ) as HTMLImageElement;
    expect(preview.src).toContain(
      "/api/media/episode-1/trickplay/trick-1/tiles/6.jpg",
    );

    fireEvent.click(screen.getByRole("button", { name: "Playback settings" }));
    const settings = await screen.findByRole("dialog", {
      name: "Playback settings",
    });
    fireEvent.click(
      within(settings).getByRole("button", { name: "Show technical stats" }),
    );
    const diagnostics = await screen.findByRole("dialog", {
      name: "Playback diagnostics",
    });
    expect(
      screen.queryByRole("dialog", { name: "Playback settings" }),
    ).not.toBeInTheDocument();
    expect(playerSurface.closest("[inert]")).not.toBeNull();
    expect(within(diagnostics).getByText("Direct Play")).toBeInTheDocument();
    fireEvent.click(
      within(diagnostics).getByRole("button", {
        name: "Close playback diagnostics",
      }),
    );
    expect(
      screen.queryByRole("dialog", { name: "Playback diagnostics" }),
    ).not.toBeInTheDocument();
    await waitFor(() => expect(playerSurface.closest("[inert]")).toBeNull());
  });

  it("defaults discovered browser captions off and renders only the selected cue in Portico", async () => {
    const source = new FixturePorticoDataSource();
    const track = new EventTarget() as TextTrack & {
      mode: TextTrackMode;
      activeCues: TextTrackCueList | null;
    };
    Object.assign(track, {
      kind: "captions",
      label: "English CC",
      language: "en",
      mode: "showing",
      activeCues: null,
    });
    const tracks = new EventTarget() as TextTrackList & {
      0: TextTrack;
      length: number;
    };
    Object.assign(tracks, { 0: track, length: 1 });
    vi.spyOn(HTMLMediaElement.prototype, "textTracks", "get").mockReturnValue(
      tracks,
    );
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback(),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());

    const view = renderPlayer(source);
    await screen.findByLabelText("Now playing The Castle");
    await waitFor(() => expect(track.mode).toBe("disabled"));
    fireEvent.click(screen.getByRole("button", { name: "Subtitles" }));
    const subtitleDialog = await screen.findByRole("dialog", {
      name: "Subtitles",
    });
    const english = within(subtitleDialog).getByRole("radio", {
      name: /English CC/,
    });
    fireEvent.click(english);
    await waitFor(() => expect(track.mode).toBe("hidden"));
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Subtitles" }),
      ).not.toBeInTheDocument(),
    );
    expect(
      screen.queryByRole("dialog", { name: "Volume" }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Subtitles" }));
    expect(
      within(await screen.findByRole("dialog", { name: "Subtitles" })).getByRole(
        "radio",
        { name: /English CC/ },
      ),
    ).toHaveAttribute("aria-checked", "true");

    Object.assign(track, {
      activeCues: {
        0: { text: "Breaking news from London" },
        length: 1,
      } as unknown as TextTrackCueList,
    });
    track.dispatchEvent(new Event("cuechange"));
    expect(
      (await screen.findByText("Breaking news from London")).closest(
        ".player-subtitle-layer",
      ),
    ).toBeInTheDocument();
    view.unmount();
    expect(track.mode).toBe("disabled");
  });

  it("does not rebuild the video source when selecting an external text subtitle", async () => {
    const source = new FixturePorticoDataSource();
    const subtitle = {
      id: "subtitle-en",
      kind: "subtitle",
      codec: "webvtt",
      language: "en",
      displayTitle: "English",
      sourceUrl: "/api/media/episode-1/subtitles/subtitle-en.vtt",
    } as PlaybackResponse["subtitleStreams"][number];
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback({ subtitleStreams: [subtitle] }),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue(queue());

    const view = renderPlayer(source);
    const surface = await screen.findByLabelText("Now playing The Castle");
    const media = surface.querySelector("video") as HTMLVideoElement;
    await waitFor(() =>
      expect(media.src).toContain("/api/media/episode-1/stream.mp4"),
    );
    const originalSource = media.src;
    const loadCalls = vi.mocked(HTMLMediaElement.prototype.load).mock.calls
      .length;
    fireEvent.click(screen.getByRole("button", { name: "Subtitles" }));
    fireEvent.click(
      within(
        await screen.findByRole("dialog", { name: "Subtitles" }),
      ).getByRole("radio", { name: /English/ }),
    );
    expect(media.src).toBe(originalSource);
    expect(HTMLMediaElement.prototype.load).toHaveBeenCalledTimes(loadCalls);
    view.unmount();
  });

  it("falls back to readable plain lyrics when synchronized timing is malformed", async () => {
    const source = new FixturePorticoDataSource();
    const musicMedia = {
      ...coreMedia("track-plain-lyrics", "Quiet Hours"),
      entityKind: "track",
      lyrics: [
        {
          id: "lyrics-broken",
          source: "embedded",
          format: "lrc",
          synced: true,
          text: "[00:99.00]Broken timing\nAll the words remain",
          createdAt: "2026-01-01T00:00:00Z",
        },
      ],
    } as PlaybackResponse["media"];
    vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
      playback({ media: musicMedia, queue: [] }),
    );
    vi.spyOn(
      source as PorticoDataSource,
      "playbackSessionQueue",
    ).mockResolvedValue({
      ...queue(),
      current: { entryId: "entry-track-plain-lyrics", media: musicMedia },
      items: [],
      history: [],
      total: 0,
    });

    renderPlayer(source, "track-plain-lyrics");
    await screen.findByLabelText("Now playing Quiet Hours");
    fireEvent.click(screen.getByRole("button", { name: "Lyrics" }));
    const lyricDialog = await screen.findByRole("dialog", { name: "Lyrics" });
    expect(
      within(lyricDialog).queryByRole("region", {
        name: "Synchronized lyrics",
      }),
    ).not.toBeInTheDocument();
    expect(within(lyricDialog).getByText(/Broken timing/)).toHaveTextContent(
      "All the words remain",
    );
    expect(
      within(lyricDialog).queryByRole("button", {
        name: "Follow current lyric",
      }),
    ).not.toBeInTheDocument();
  });

  it.each([
    ["track", "track-1", "Night Drive", "mode-music"],
    ["audiobook", "book-1", "The Long Way Home", "mode-audiobook"],
  ] as const)(
    "renders %s playback with its audio-first artwork mode",
    async (entityKind, id, title, className) => {
      const source = new FixturePorticoDataSource();
      const media = {
        ...coreMedia(id, title),
        entityKind,
      } as PlaybackResponse["media"];
      vi.spyOn(source as PorticoDataSource, "startPlayback").mockResolvedValue(
        playback({ media, queue: [] }),
      );
      vi.spyOn(
        source as PorticoDataSource,
        "playbackSessionQueue",
      ).mockResolvedValue({
        ...queue(),
        current: { entryId: `entry-${media.id}`, media },
        items: [],
        history: [],
        total: 0,
      });

      renderPlayer(source, id);
      const surface = await screen.findByLabelText(`Now playing ${title}`);
      expect(surface).toHaveClass(className);
      expect(surface.querySelector(".audio-artwork img")).toHaveAttribute(
        "src",
        "http://localhost:3000/poster.jpg",
      );
    },
  );
});
