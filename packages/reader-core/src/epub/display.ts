import {ReaderError} from '../errors.ts';
import {spreadFits, compactLandscape, validateViewport} from '../layout.ts';
import type {ReaderViewport} from '../ports.ts';
/** Narrow engine input. This is structurally satisfied by the canonical fifteen
 * property Reading facade; no defaults, raw registry keys or write API here. */
export type EpubDisplayPreferences = Readonly<{
  theme: 'dark' | 'black' | 'sepia' | 'light'; font: 'publisher' | 'literata' | 'manrope';
  textSize: number; lineSpacing: 'tight' | 'normal' | 'loose'; margins: 'narrow' | 'normal' | 'wide';
  alignment: 'left' | 'justified'; layout: 'pages' | 'scroll'; twoPages: boolean;
  hyphenation: boolean; tapToTurn: boolean; highlightColour: 'yellow' | 'green' | 'blue' | 'pink';
}>;
const palettes = {dark: ['#0c151d','#d4dde4'], black: ['#000000','#c9cfd3'], sepia: ['#f1e6d1','#443629'], light: ['#fbf9f5','#1d252c']} as const;
const colours = {yellow:'#d2b45b', green:'#75ad86', blue:'#75a9cf', pink:'#c88daa'} as const;
export function epubDisplay(preferences: EpubDisplayPreferences, viewport: ReaderViewport, fonts: Readonly<{literata: string; manrope: string}>) {
  validateViewport(viewport);
  const p = preferences;
  if (!palettes[p.theme] || !['publisher','literata','manrope'].includes(p.font) || !Number.isInteger(p.textSize) || p.textSize<1 || p.textSize>7
    || !['tight','normal','loose'].includes(p.lineSpacing) || !['narrow','normal','wide'].includes(p.margins)
    || !['left','justified'].includes(p.alignment) || !['pages','scroll'].includes(p.layout) || !colours[p.highlightColour]
    || [p.twoPages,p.hyphenation,p.tapToTurn].some(v => typeof v !== 'boolean')) throw new ReaderError('invalid_preferences');
  const [background, ink] = palettes[p.theme], size = [14,16,17,18,20,23,26][p.textSize-1];
  const lineHeight = {tight: 1.4, normal: 1.6, loose: 1.85}[p.lineSpacing];
  const normalMargin = viewport.width < 600 ? 28 : viewport.width < 1024 ? 84 : viewport.width >= 1400 ? 150 : 76;
  const margin = normalMargin * {narrow:0.65,normal:1,wide:1.35}[p.margins];
  const columns = p.layout === 'pages' && p.twoPages && (spreadFits(viewport) || compactLandscape(viewport)) ? 2 : 1;
  const top = viewport.width < 600 ? 100 : 100;
  // Trusted font URLs are created from app-supplied font Blobs, never book text.
  const fontFaces = `@font-face{font-family:Literata;src:url("${fonts.literata}")}@font-face{font-family:Manrope;src:url("${fonts.manrope}")}`;
  const family = p.font === 'publisher' ? '' : `font-family:${p.font === 'literata' ? 'Literata,serif' : 'Manrope,sans-serif'}!important;`;
  const css = `${fontFaces}\nhtml{color:${ink}!important;background:${background}!important;color-scheme:${p.theme==='dark'||p.theme==='black'?'dark':'light'};}
body{${family}font-size:${size}px!important;line-height:${lineHeight}!important;text-align:${p.alignment==='justified'?'justify':'left'}!important;hyphens:${p.hyphenation?'auto':'none'}!important;-webkit-hyphens:${p.hyphenation?'auto':'none'}!important;color:${ink}!important;background:transparent!important;font-optical-sizing:auto;}
p{margin:0;text-indent:1.6em;}p:first-of-type{ text-indent:0; }
::selection{background:${colours[p.highlightColour]}88;}
a{color:inherit;}img,svg{max-width:100%;max-height:100%;}*{animation:none!important;transition:none!important;}`;
  return Object.freeze({background, ink, columns, margin, top, css, flow:p.layout==='pages'?'paginated':'scrolled',
    maxInlineSize: (viewport.width - 2*margin) / columns, gap: 100 * (2*margin) / viewport.width});
}
