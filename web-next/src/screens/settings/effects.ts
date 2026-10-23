import {settingsBinding, type SettingsBinding} from '@core/presentation/settings-bindings.ts';
import {settingsRowIndex, type SettingsRow} from '@core/presentation/settings-spec.ts';

/** Availability of actual MAIN consumers, not a storage/binding/default map.
 * Home/Browse/Saved consume the same confirmed appearance facade; Show owns
 * episode layout and bootstrap applies the system-motion intersection. */
const preferenceEffects = new Set([
  'playback.defaultSpeed', 'playback.introSkip', 'playback.creditsSkip', 'playback.recapSkip',
  'playback.passoutProtection', 'playback.passoutAfterEpisodes', 'playback.skipBackMs', 'playback.skipForwardMs', 'audiobooks.defaultSpeed',
  'accessibility.reducedMotion', 'appearance.motion', 'appearance.showBackdrops', 'appearance.cardSizePercent',
  'appearance.libraryView', 'appearance.episodesLayout', 'notifications.badges',
]);
function preferenceEffect(binding: SettingsBinding): boolean {
  if (preferenceEffects.has(binding.key) || binding.key.startsWith('subtitles.')) return true;
  const parts = Object.values(binding.fields ?? {});
  return parts.length > 0 && parts.every(key => preferenceEffects.has(key));
}
/** A composite/preset is offered only when every canonical target has a real
 * effect. Unsupported native outputs and Library eligibility remain separate. */
export function hasWebPreferenceEffect(row: SettingsRow, visited: ReadonlySet<string> = new Set()): boolean {
  if (visited.has(row.id)) return false;
  const next = new Set([...visited, row.id]);
  const presets = row.control.options?.filter(option => option.sets);
  if (presets?.length) {
    const targets = [...new Set(presets.flatMap(option => Object.keys(option.sets!)))];
    return targets.length > 0 && targets.every(id => {const target = settingsRowIndex.get(id)?.row; return !!target && hasWebPreferenceEffect(target, next);});
  }
  const binding = settingsBinding(row.id);
  return binding?.form === 'preferences' && preferenceEffect(binding);
}
