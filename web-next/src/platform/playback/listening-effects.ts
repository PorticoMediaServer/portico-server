import type {PlaybackService} from '@core/index.ts';
export type {ListeningNoticeCode, AudioProcessingBinding, AudioProcessingApplied as AppliedAudioProcessing} from '@core/audio-effects.ts';
/** These required methods are the checked Music producer, with its exact current-session guard. */
export type ListeningEffectsPort = Pick<PlaybackService, 'listeningNotice' | 'audioProcessingApplied' | 'audioPreparationFailed'>;

/** Only diagnostic codes leave this helper. It never returns presentation text. */
export function listeningDiagnostic(code: 'audio_transition_unavailable' | 'audio_edge_late' | 'audio_processing_unavailable' | 'equalizer_filter_unavailable', error?: unknown): void {
  console.debug(code, error instanceof Error ? error.name : undefined);
}
