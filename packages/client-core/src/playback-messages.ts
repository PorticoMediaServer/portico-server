import {serviceText} from './presentation/service-text.ts';
import {sessionEndMessage} from './session-end.ts';
export {sessionEndMessage};
/** Presentation only: recovery and authorization decisions still use the original failure. */
export function playbackUserMessage(error:unknown):string {
 const value=error&&typeof error==='object'?error as {code?:unknown;message?:unknown;status?:unknown}:undefined;
 const code=typeof value?.code==='string'?value.code:typeof error==='string'?error:'';
 const messages:Record<string,string>={
  unauthorized:'Sign in again to continue playback.',access_revoked:'Your access to this media has changed.',forbidden:'Your profile does not have access to this media.',
  source_unavailable:'This media source is unavailable. Try again when it reconnects.',source_changed:'The media file changed. Open the title again to continue.',not_found:'This media is no longer available.',
  owner_account_cap:'Your account has reached its simultaneous stream limit. Stop another stream and try again.',owner_server_cap:'This server has reached its simultaneous stream limit. Try again when another stream finishes.',
  transcoding_disabled:'This file needs conversion, which is disabled on this server.',unsupported_tuple:'This device cannot play the available format.',unknown_source_facts:'The server could not read this file’s playback details.',
  encoder_failure:'The server could not convert this file for playback.',actual_allocation_failure:'The server does not have enough resources to start this stream.',decoder_contradiction:'This device could not play the stream. Try another available quality or version.',
  delivery_http_failure:'The stream could not be loaded. Check your connection and try again.',network_stall:'Playback is waiting for the connection to recover.',timeout:'The server took too long to respond. Try again.',
  controller_retired:'Playback moved to another device. Open this title again to play here.',lease_expired:'The stream disconnected. Retry to reconnect.',candidate_expired:'These playback options have expired. Open them again.',
  revision_conflict:'Playback changed on another device. Refresh before trying again.',identity_conflict:'The playback session changed. Open this title again.',sequence_stale:'Playback changed on another device. Refresh before trying again.',
  receipt_expired:'The last playback action could not be confirmed. Check playback before trying again.',receipt_not_found:'The last playback action could not be confirmed. Check playback before trying again.',
  lane_occupied:'Another playback action is still finishing. Please try again shortly.',operation_unavailable:'This playback action is not available right now.',seek_mismatch:'Playback could not reach that position. Try seeking again.',timeline_unmapped:'That position is outside the available programme. Choose Go Live.',
  unsupported_version:'Update this app to play media from this server.',invalid_request:'This playback action could not be completed. Open the title again.',internal_error:'Playback could not start. Please try again.'
 };
 if(code==='terminated')return sessionEndMessage({reason:'terminated',message:typeof value?.message==='string'?value.message:undefined});
 if(code==='queue_too_large')return serviceText('error.queueTooLarge');
 // Lyric and subtitle searches on a library whose metadata source is local only.
 if(code==='local_metadata_only')return serviceText('error.localMetadataOnly');
 if(messages[code])return messages[code];
 if(value?.status===401)return messages.unauthorized;
 if(value?.status===403)return messages.forbidden;
 // Do not echo arbitrary server text, URLs, IDs, native exceptions or serialized payloads.
 const message=typeof error==='string'?error:typeof value?.message==='string'?value.message:'';
 const safe=[/^This title can’t be played on this device\.$/,/^This title needs converting, and converting is turned off for this account\.$/,/^This item isn’t available\.$/,/^This account is already playing as many streams as it is allowed\.$/,/^This queue is too long\. Remove some of it or start a new one\.$/,/^This part of the queue is still loading\. Try again in a moment\.$/,/^The queue has ended\.$/,/^Listening timer paused playback\.$/,/^Playback has ended on the server\.$/,/^Choose Go Live or a position in the retained window before resuming\.$/,/^The channel stream ended\. Retry or choose Go Live\.$/,/^The previous playback could not be stopped\. Retry this channel when the server reconnects\.$/];
 if(message==='Playback control disconnected. Retrying…')return 'Reconnecting playback controls…';
 if(message==='This media could not be played. Your playback position is retained.')return 'This device could not play the stream. Your position is saved; try again.';
 if(safe.some(pattern=>pattern.test(message)))return message;
 if(error instanceof TypeError||/network|fetch|connection|timed? ?out/i.test(message))return 'The stream could not connect. Check your connection and try again.';
 return 'Playback could not continue. Please try again.';
}

/**
 * Whether trying again cannot help: the title cannot be played on this device at all, or this
 * viewer may not play it. The player then says so plainly instead of offering "Try again" as
 * the way forward. Everything else (the network, a busy server, a stream that was ended) is
 * worth another try.
 */
export function playbackFailurePermanent(error:unknown):boolean {
 const value=error&&typeof error==='object'?error as {code?:unknown;message?:unknown}:undefined;
 const text=typeof error==='string'?error:typeof value?.code==='string'?value.code:typeof value?.message==='string'?value.message:'';
 return ['cannot_play_here','streaming_unsupported','unsupported_media','unsupported_tuple','unknown_source_facts','transcoding_disabled','transcode_not_allowed','not_found','forbidden','access_revoked','unsupported_version',
  'This title can’t be played on this device.','This title needs converting, and converting is turned off for this account.','This item isn’t available.'].includes(text);
}
