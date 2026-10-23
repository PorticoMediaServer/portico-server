/** Music-owned core contract proposal, b4 listening-notices. Optional only until its checked
 * producer is integrated by Root; there is deliberately no legacy text-notice fallback. */
export type ListeningNoticeCode = 'audio_autoplay_blocked' | 'listening_policy_failed';
export type AudioProcessingBinding = Readonly<{intentId: number; sessionId: string; generation: number}>;
export type AppliedAudioProcessing = Readonly<{eqApplied: boolean; levelingApplied: boolean}>;
export type ListeningEffectsPort = Readonly<{
  listeningNotice?(code: ListeningNoticeCode): void;
  audioProcessingApplied?(binding: AudioProcessingBinding, facts: AppliedAudioProcessing): boolean;
  audioPreparationFailed(): void;
}>;

/** Only diagnostic codes leave this helper. It never returns presentation text. */
export function listeningDiagnostic(code: 'audio_transition_unavailable' | 'audio_edge_late' | 'equalizer_filter_unavailable', error?: unknown): void {
  console.debug(code, error instanceof Error ? error.name : undefined);
}
