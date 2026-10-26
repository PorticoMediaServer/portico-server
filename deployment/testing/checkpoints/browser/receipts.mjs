/** Bounded checkpoint receipts. Every field is a code, count or closed enum:
 * never a URL, query, nonce, body, token, message or account identifier. */
export const ERROR_CLASSES=new Set(['Error','TypeError','ReferenceError','SyntaxError','SecurityError','AbortError','NotSupportedError','InvalidStateError','NetworkError','EncodingError','NotAllowedError','DOMException']);

const OPERATIONS=new Map([['/v1/sessions','sessions'],['/v1/sessions/refresh','refresh'],['/v1/account','account'],['/v1/account/security','security'],['/v1/account/sessions','devices'],['/v1/servers','directory'],['/v1/playback/sessions','playback-start'],['/v1/search','search'],['/v1/queues','queue-create'],['/v1/playback/controls','playback-controls']]);
const READINGS=new Map([['/v1/reading/items','reading-items'],['/v1/reading/continue','reading-continue'],['/v1/reading/discover','reading-discover'],['/v1/reading/search','reading-search'],['/v1/reading/sessions','reading-sessions']]);
const ERROR_CODES=new Set(['invalid_credentials','session_mode_denied','origin_denied','invalid_request','forbidden','source_unavailable','internal','metadata_conflict','challenge_invalid','rate_limited','session_expired','stream_limit_reached','conversion_capacity','transcode_not_allowed','unsupported_media','connection_too_slow','queue_building','not_permitted','no_tuner_available','revision_mismatch','revision_conflict','revision_required','request_in_progress','body_too_large','not_found','unauthorized']);
const READING_FORMATS=new Set(['epub','pdf','comic']);
const CSP_DIRECTIVES=new Set(['default-src','style-src','style-src-elem','style-src-attr','script-src','script-src-elem','script-src-attr','font-src','connect-src','img-src','frame-src','media-src']);
const CSP_BLOCKED_CLASSES=new Set(['inline','eval','blob','data','same-origin','foreign-origin','other']);

/** Named product operation for a path, or null when the path is not receipted.
 * Reading keeps its closed staged operations: session create and close, progress,
 * source, pages, annotations, search, assets and file-save. */
export function operation(pathname){
 const known=OPERATIONS.get(pathname);
 if(known)return known;
 const reading=READINGS.get(pathname);
 if(reading)return reading;
 if(/^\/v1\/items\/[^/]+\/playback-options$/.test(pathname))return 'playback-options';
 if(/^\/v1\/reading\/sessions\/[^/]+\/progress$/.test(pathname))return 'reading-progress';
 if(/^\/v1\/reading\/sessions\/[^/]+\/source$/.test(pathname))return 'reading-source';
 if(/^\/v1\/reading\/sessions\/[^/]+\/pages\/[^/]+$/.test(pathname))return 'reading-pages';
 if(/^\/v1\/reading\/sessions\/[^/]+\/annotations$/.test(pathname))return 'reading-annotations';
 if(/^\/v1\/reading\/sessions\/[^/]+\/search$/.test(pathname))return 'reading-search';
 if(/^\/v1\/reading\/sessions\/[^/]+\/assets$/.test(pathname))return 'reading-assets';
 if(/^\/v1\/reading\/sessions\/[^/]+\/file-save$/.test(pathname))return 'reading-file-save';
 if(/^\/v1\/reading\/sessions\/[^/]+$/.test(pathname))return 'reading-close';
 return null;
}

export function readingFormat(value){return READING_FORMATS.has(value)?value:undefined;}

/** Closed product error code from an already-parsed error body, never the body. */
export function errorCode(data){const code=data?.error?.code??data?.code;return ERROR_CODES.has(code)?code:undefined;}

/** Main-frame page error receipt: class, closed category and code coordinates. */
export function pageError(error,phase){
 const text=String(error?.message??'');
 const code=/dynamically imported module/i.test(text)?'import_failed':/play\(\).*user|user.*interact|play.*not allowed/i.test(text)?'play_not_allowed':/no supported source|source.*not supported/i.test(text)?'unsupported_source':/Cannot read properties of (?:null|undefined)/.test(text)?'null_property':/Minified React error #\d+/.test(text)?'react_error':'other';
 return {phase,class:ERROR_CLASSES.has(error?.name)?error.name:'Other',code,coordinates:[...String(error?.stack??'').matchAll(/:(\d+):(\d+)/g)].slice(0,6).map(match=>[Number(match[1]),Number(match[2])])};
}

/** Installed as an init script, so this function must stay self-contained. */
export function installCspRecorder(){
 window.__porticoCheckpointCsp=[];
 addEventListener('securitypolicyviolation',event=>{
  const allowed=['default-src','style-src','style-src-elem','style-src-attr','script-src','script-src-elem','script-src-attr','font-src','connect-src','img-src','frame-src','media-src'];
  if(!allowed.includes(event.effectiveDirective)||window.__porticoCheckpointCsp.length>=32)return;
  let blockedClass=['inline','eval'].includes(event.blockedURI)?event.blockedURI:'other';
  try{const url=new URL(event.blockedURI);blockedClass=['blob:','data:'].includes(url.protocol)?url.protocol.slice(0,-1):url.origin===location.origin?'same-origin':'foreign-origin';}catch{}
  window.__porticoCheckpointCsp.push({directive:event.effectiveDirective,blockedClass});
 });
}

/** Directive and closed blocked-class counts only; the blocked URI never
 * leaves the page. */
export function cspSummary(entries){
 const directives={},blockedClasses={},bounded=Array.isArray(entries)?entries.slice(0,32):[];
 for(const entry of bounded){
  const directive=CSP_DIRECTIVES.has(entry?.directive)?entry.directive:'other';
  const blockedClass=CSP_BLOCKED_CLASSES.has(entry?.blockedClass)?entry.blockedClass:'other';
  directives[directive]=(directives[directive]??0)+1;
  blockedClasses[blockedClass]=(blockedClasses[blockedClass]??0)+1;
 }
 return {total:bounded.length,directives,blockedClasses};
}
