/** Source-owned label parts. The stable HLS NAME/label is a transport identity;
 * it is intentionally never parsed or used as the menu's display model. */
export type VideoTrackLabelParts = Readonly<{
 title?: string;
 language?: string;
 format: string;
 objectAudio?: 'atmos' | 'dtsx';
 layout?: string;
 origin?: 'embedded' | 'sidecar' | 'provider' | 'upload' | 'downloaded';
 label?: string;
}>;

/** Catalogue additions are owned by the media catalogue lane. These keys use
 * only the exact VS3e phrases; codec/layout names are values, not translations. */
export type VideoTrackTextKey =
 | 'player.info.track.withAtmos'
 | 'player.info.track.withDtsx'
 | 'player.info.track.besideFile'
 | 'player.info.track.downloaded';
export type VideoTrackWords = Readonly<{
 languageName(tag: string): string | undefined;
 text(key: VideoTrackTextKey, params?: Readonly<Record<string, string>>): string;
}>;
export type VideoTrackName = Readonly<{primary: string; secondary: string; fullName: string}>;

const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const boundedText = (v: unknown, limit: number): v is string => typeof v === 'string' && v.length <= limit && !/[\u0000-\u001f\u007f]/u.test(v);

/** Parse only the actual label-parts record, never a rendition object with URLs. */
export function parseVideoTrackLabelParts(raw: unknown): VideoTrackLabelParts {
 if (!object(raw) || Object.keys(raw).some(k => !['title','language','format','objectAudio','layout','origin','label'].includes(k)) || !boundedText(raw.format,64)) throw new TypeError('Invalid video track label parts');
 for (const [key,limit] of [['title',96],['language',32],['layout',64],['label',512]] as const) if (raw[key] !== undefined && !boundedText(raw[key],limit)) throw new TypeError('Invalid video track label parts');
 if (raw.objectAudio !== undefined && raw.objectAudio !== 'atmos' && raw.objectAudio !== 'dtsx') throw new TypeError('Invalid video track objects');
 if (raw.origin !== undefined && !['embedded','sidecar','provider','upload','downloaded'].includes(String(raw.origin))) throw new TypeError('Invalid subtitle origin');
 return Object.freeze({format:raw.format,...(raw.title ? {title:raw.title as string} : {}),...(raw.language ? {language:raw.language as string} : {}),...(raw.layout ? {layout:raw.layout as string} : {}),...(raw.objectAudio ? {objectAudio:raw.objectAudio as 'atmos'|'dtsx'} : {}),...(raw.origin ? {origin:raw.origin as VideoTrackLabelParts['origin']} : {}),...(raw.label !== undefined ? {label:raw.label as string} : {})});
}

/** The menu header uses primary, its detail line uses secondary, and one-line
 * or accessibility surfaces use fullName. Missing facts stay absent. */
export function videoTrackName(parts: VideoTrackLabelParts, words: VideoTrackWords): VideoTrackName {
 const language=parts.language && !['und','xx'].includes(parts.language.toLowerCase()) ? parts.language : undefined;
 const head=parts.title || (language ? words.languageName(language) || language : '');
 let format=parts.format;
 if (parts.objectAudio === 'atmos') format=words.text('player.info.track.withAtmos',{format});
 let layout=parts.layout ?? '';
 if (parts.objectAudio === 'dtsx') {
  if (layout) layout=words.text('player.info.track.withDtsx',{value:layout});
  else format=words.text('player.info.track.withDtsx',{value:format});
 }
 const detail=[format,layout].filter(Boolean);
 if (parts.origin === 'sidecar') detail.push(words.text('player.info.track.besideFile'));
 if (parts.origin === 'provider' || parts.origin === 'downloaded') detail.push(words.text('player.info.track.downloaded'));
 // An upload is not called downloaded. Its origin remains available to any
 // drawn surface that has approved upload wording.
 const primary=head || format;
 const secondary=(head ? detail : detail.slice(1)).join(', ');
 return Object.freeze({primary,secondary,fullName:[primary,secondary].filter(Boolean).join(', ')});
}
