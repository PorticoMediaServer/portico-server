/**
 * The server owner stopped this playback (spec §14): read from `PlaybackSnapshot.ended`, which
 * client-core sets for v1 sessions (a forwarded `session.updated`, or a timeline that finds the
 * session gone) and v2 occurrences (terminal error `terminated`) alike. Never retried.
 */
import {sessionEndMessage} from '@core/session-end.ts';

export type OwnerStop = Readonly<{message?: string}>;
type Ended = Readonly<{reason: string; message?: string}> | undefined;

/** The owner's stop and their message (cleaned by core), or undefined for any other end. */
export function ownerStopOf(ended: Ended): OwnerStop | undefined {
  if (ended?.reason !== 'terminated') return undefined;
  return Object.freeze(ended.message?.trim() ? {message: sessionEndMessage(ended)} : {});
}
