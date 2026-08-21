export type NativePlaybackMediaKind = "video" | "music" | "audiobook" | "live";

export type NativePlaybackShellCapabilities = {
  pictureInPicture: boolean;
  nowPlaying: boolean;
  backgroundAudio: boolean;
};

export type NativePlaybackCoordinationPlan = {
  mediaKind: NativePlaybackMediaKind;
  allowPictureInPicture: boolean;
  publishNowPlaying: boolean;
  allowBackgroundAudio: boolean;
  interruptionBehavior: "pause-and-report";
  remoteCommands: readonly ("play" | "pause" | "seek" | "previous" | "next" | "stop")[];
};

/** Native-agnostic plan for AVKit/MediaPlayer adapters; no OS capability is guessed. */
export function nativePlaybackCoordinationPlan(
  mediaKind: NativePlaybackMediaKind,
  capabilities: NativePlaybackShellCapabilities
): NativePlaybackCoordinationPlan {
  const audioContinuity = mediaKind === "music" || mediaKind === "audiobook";
  return {
    mediaKind,
    allowPictureInPicture: capabilities.pictureInPicture && (mediaKind === "video" || mediaKind === "live"),
    publishNowPlaying: capabilities.nowPlaying,
    allowBackgroundAudio: capabilities.backgroundAudio && audioContinuity,
    interruptionBehavior: "pause-and-report",
    remoteCommands: mediaKind === "live"
      ? ["play", "pause", "stop"]
      : ["play", "pause", "seek", "previous", "next", "stop"]
  };
}

export type PortableSleepTimer =
  | { mode: "off" }
  | { mode: "end-of-item" }
  | { mode: "deadline"; deadlineAt: number };

export function portableSleepTimer(mode: "off" | "end-of-item" | 15 | 30 | 45 | 60, now = Date.now()): PortableSleepTimer {
  if (mode === "off") return { mode: "off" };
  if (mode === "end-of-item") return { mode: "end-of-item" };
  return { mode: "deadline", deadlineAt: now + mode * 60_000 };
}

export function sleepTimerShouldStop(timer: PortableSleepTimer, event: { type: "tick"; now: number } | { type: "item-ended" }): boolean {
  if (timer.mode === "off") return false;
  if (timer.mode === "end-of-item") return event.type === "item-ended";
  return event.type === "tick" && event.now >= timer.deadlineAt;
}
