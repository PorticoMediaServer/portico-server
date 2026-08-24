import {
  reduceWatchWithFriendsSnapshot,
  type WatchWithFriendsGroup,
  type WatchWithFriendsSyncState,
} from "@porticomediaserver/client-core";
import { useCallback, useEffect, useRef } from "react";
import { useOptionalPlaybackSession } from "../player/PlayerSurface";
import {
  groupIncludesViewer,
  viewerCanHost,
  type WatchWithFriendsSource,
  type WatchWithFriendsViewer,
} from "./watchWithFriendsSource";

type MemberPlaybackState = "ready" | "buffering" | "playing" | "paused";

interface WatchWithFriendsPlaybackSyncOptions {
  group?: WatchWithFriendsGroup;
  source: WatchWithFriendsSource;
  viewer: WatchWithFriendsViewer;
}

function currentPosition(media?: HTMLVideoElement | null) {
  return media && Number.isFinite(media.currentTime)
    ? Math.max(0, Math.floor(media.currentTime))
    : 0;
}

export function useWatchWithFriendsPlaybackSync({
  group,
  source,
  viewer,
}: WatchWithFriendsPlaybackSyncOptions) {
  const player = useOptionalPlaybackSession();
  const playerRef = useRef(player);
  playerRef.current = player;
  const groupRef = useRef<WatchWithFriendsGroup | undefined>(undefined);
  const groupIdRef = useRef("");
  const lastCommandIdRef = useRef("");
  const syncStateRef = useRef<WatchWithFriendsSyncState | undefined>(undefined);
  const applyingGroupStateRef = useRef(false);
  const releaseTimerRef = useRef<number | undefined>(undefined);
  const seekTimerRef = useRef<number | undefined>(undefined);
  const rateTimerRef = useRef<number | undefined>(undefined);
  const lastMemberReportRef = useRef<
    { state: MemberPlaybackState; position: number; at: number } | undefined
  >(undefined);
  const applyGenerationRef = useRef(0);
  const applyChainRef = useRef<Promise<void>>(Promise.resolve());
  const hostMutationChainRef = useRef<Promise<void>>(Promise.resolve());

  useEffect(() => {
    const nextGroupId = group?.id ?? "";
    if (groupIdRef.current !== nextGroupId) {
      groupIdRef.current = nextGroupId;
      lastCommandIdRef.current = "";
      syncStateRef.current = undefined;
      lastMemberReportRef.current = undefined;
      applyingGroupStateRef.current = false;
      applyGenerationRef.current += 1;
    }
    groupRef.current = group;
    if (!group) {
      lastCommandIdRef.current = "";
      syncStateRef.current = undefined;
    }
  }, [group]);

  const reportMemberState = useCallback(
    (state: MemberPlaybackState) => {
      const currentGroup = groupRef.current;
      const currentPlayer = playerRef.current;
      if (
        !currentGroup ||
        !currentPlayer ||
        !groupIncludesViewer(currentGroup, viewer)
      )
        return;
      const position = currentPosition(currentPlayer.mediaRef.current);
      const now = Date.now();
      const previous = lastMemberReportRef.current;
      if (
        previous &&
        previous.state === state &&
        Math.abs(previous.position - position) < 2 &&
        now - previous.at < 2_000
      )
        return;
      lastMemberReportRef.current = { state, position, at: now };
      void source
        .updateMemberState(currentGroup.id, {
          state,
          positionSeconds: position,
        })
        .catch(() => undefined);
    },
    [source, viewer],
  );

  const applyGroupUpdate = useCallback(
    (nextGroup: WatchWithFriendsGroup) => {
      if (groupIdRef.current !== nextGroup.id) {
        groupIdRef.current = nextGroup.id;
        lastCommandIdRef.current = "";
        syncStateRef.current = undefined;
        lastMemberReportRef.current = undefined;
        applyGenerationRef.current += 1;
      }
      groupRef.current = nextGroup;
      const currentPlayer = playerRef.current;
      if (!currentPlayer || !groupIncludesViewer(nextGroup, viewer)) return;
      const media = currentPlayer.mediaRef.current;
      if (nextGroup.state === "stopped") {
        void currentPlayer.applyExternalCommand({
          ...nextGroup.command,
          action: "stop",
        });
        return;
      }
      const result = reduceWatchWithFriendsSnapshot(
        syncStateRef.current,
        nextGroup,
        {
          mediaId: currentPlayer.playback?.media.id,
          positionSeconds: currentPosition(media),
          paused: media?.paused ?? true,
          buffering:
            currentPlayer.status === "buffering" ||
            currentPlayer.status === "recovering",
          playbackRate: media?.playbackRate ?? 1,
        },
      );
      syncStateRef.current = result.state;
      if (result.action.type === "ignore" || result.action.type === "none")
        return;

      applyingGroupStateRef.current = true;
      if (releaseTimerRef.current !== undefined)
        window.clearTimeout(releaseTimerRef.current);
      const generation = ++applyGenerationRef.current;
      const stillCurrent = () =>
        generation === applyGenerationRef.current &&
        groupRef.current?.id === nextGroup.id;
      const synchronize = async () => {
        if (!stillCurrent()) return;
        const action = result.action;
        if (action.type === "load") {
          await currentPlayer.applyExternalCommand({
            action: "load",
            mediaId: action.mediaId,
            positionSeconds: action.positionSeconds,
            issuedAt: nextGroup.serverTime,
          });
          if (!stillCurrent()) return;
          await currentPlayer.applyExternalCommand({
            action: action.paused ? "pause" : "play",
            positionSeconds: action.positionSeconds,
            issuedAt: nextGroup.serverTime,
          });
        }
        if (action.type === "seek") {
          await currentPlayer.applyExternalCommand({
            action: "seek",
            positionSeconds: action.positionSeconds,
            issuedAt: nextGroup.serverTime,
          });
          if (!stillCurrent()) return;
          await currentPlayer.applyExternalCommand({
            action: action.paused ? "pause" : "play",
            positionSeconds: action.positionSeconds,
            issuedAt: nextGroup.serverTime,
          });
        }
        if (action.type === "play" || action.type === "pause")
          await currentPlayer.applyExternalCommand({
            action: action.type,
            positionSeconds: result.targetPositionSeconds,
            issuedAt: nextGroup.serverTime,
          });
        if (action.type === "rate" && media) {
          if (rateTimerRef.current !== undefined)
            window.clearTimeout(rateTimerRef.current);
          media.playbackRate = action.playbackRate;
          rateTimerRef.current = window.setTimeout(() => {
            media.playbackRate = nextGroup.playbackRate || 1;
          }, action.durationMs);
        }
      };
      applyChainRef.current = applyChainRef.current
        .catch(() => undefined)
        .then(synchronize);
      void applyChainRef.current.finally(() => {
        if (!stillCurrent()) return;
        releaseTimerRef.current = window.setTimeout(() => {
          if (stillCurrent()) applyingGroupStateRef.current = false;
        }, 350);
      });
    },
    [viewer],
  );

  useEffect(() => {
    if (group) applyGroupUpdate(group);
  }, [applyGroupUpdate, group]);

  useEffect(() => {
    const media = player?.mediaRef.current;
    const currentGroup = groupRef.current;
    if (!media || !currentGroup || !groupIncludesViewer(currentGroup, viewer))
      return;

    const updateHostTransport = (action: "play" | "pause" | "seek") => {
      const requestedGroupId = groupRef.current?.id;
      if (!requestedGroupId) return;
      hostMutationChainRef.current = hostMutationChainRef.current
        .catch(() => undefined)
        .then(async () => {
          const activeGroup = groupRef.current;
          if (
            !activeGroup ||
            activeGroup.id !== requestedGroupId ||
            applyingGroupStateRef.current ||
            !viewerCanHost(activeGroup, viewer)
          )
            return;
          try {
            applyGroupUpdate(
              await source.updatePlaybackState(activeGroup.id, {
                action,
                positionSeconds: currentPosition(media),
                playbackRate: media.playbackRate,
                expectedRevision: activeGroup.revision,
                idempotencyKey: globalThis.crypto.randomUUID(),
              }),
            );
          } catch {
            try {
              if (groupRef.current?.id === activeGroup.id)
                applyGroupUpdate(await source.group(activeGroup.id));
            } catch {
              applyingGroupStateRef.current = false;
            }
          }
        });
    };
    const onPlay = () => {
      reportMemberState("playing");
      updateHostTransport("play");
    };
    const onPause = () => {
      reportMemberState("paused");
      updateHostTransport("pause");
    };
    const onWaiting = () => reportMemberState("buffering");
    const onPlaying = () => reportMemberState("playing");
    const onSeeked = () => {
      reportMemberState(media.paused ? "paused" : "playing");
      if (seekTimerRef.current !== undefined)
        window.clearTimeout(seekTimerRef.current);
      seekTimerRef.current = window.setTimeout(
        () => updateHostTransport("seek"),
        180,
      );
    };

    reportMemberState(media.paused ? "ready" : "playing");
    media.addEventListener("play", onPlay);
    media.addEventListener("pause", onPause);
    media.addEventListener("waiting", onWaiting);
    media.addEventListener("playing", onPlaying);
    media.addEventListener("seeked", onSeeked);
    return () => {
      media.removeEventListener("play", onPlay);
      media.removeEventListener("pause", onPause);
      media.removeEventListener("waiting", onWaiting);
      media.removeEventListener("playing", onPlaying);
      media.removeEventListener("seeked", onSeeked);
      if (seekTimerRef.current !== undefined)
        window.clearTimeout(seekTimerRef.current);
    };
  }, [
    applyGroupUpdate,
    player?.playback?.sessionId,
    reportMemberState,
    source,
    viewer,
  ]);

  useEffect(
    () => () => {
      if (releaseTimerRef.current !== undefined)
        window.clearTimeout(releaseTimerRef.current);
      if (seekTimerRef.current !== undefined)
        window.clearTimeout(seekTimerRef.current);
      if (rateTimerRef.current !== undefined)
        window.clearTimeout(rateTimerRef.current);
    },
    [],
  );

  return { applyGroupUpdate };
}
