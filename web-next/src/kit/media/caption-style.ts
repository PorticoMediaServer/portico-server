import type {SubtitlePreferences} from '@core/preference-effects.ts';

/** The canonical app-caption preferences, independent of the app theme. */
export type CaptionAppearance = Pick<SubtitlePreferences, 'size' | 'color' | 'background' | 'font'>;
/** key is created internally by SubtitleSurface, never provided by a user or a subtitle. */
export function captionStyle(key: string, preferences: CaptionAppearance): string {
  const sizes = {small:'80%',medium:'100%',large:'125%','extra-large':'150%'};
  const colors = {white:'#fff',yellow:'#ff0',cyan:'#0ff',green:'#0f0',pink:'#ff80c0'};
  const backgrounds = {none:'transparent',outline:'transparent',shadow:'transparent',translucent:'rgba(0,0,0,0.65)',opaque:'#000'};
  const fonts = {default:'sans-serif',serif:'serif',mono:'monospace',rounded:'ui-rounded,system-ui,sans-serif'};
  const shadows = {none:'none',outline:'-1px -1px 0 #000,1px -1px 0 #000,-1px 1px 0 #000,1px 1px 0 #000',shadow:'0 2px 3px #000',translucent:'none',opaque:'none'};
  return `[data-portico-caption-style="${key}"]::cue {font-size:${sizes[preferences.size]};color:${colors[preferences.color]};background-color:${backgrounds[preferences.background]};font-family:${fonts[preferences.font]};text-shadow:${shadows[preferences.background]};}`;
}
