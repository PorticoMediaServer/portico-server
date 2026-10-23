import DOMPurify from 'dompurify';
import {ReaderError} from '../errors.ts';

// A blob inherits the app's origin. Foliate requires same-origin DOM access,
// and WebKit requires allow-scripts for events: enforce this CSP in the book
// itself BEFORE the browser sees any untrusted element. No book network access.
export const BOOK_CSP = "default-src 'none'; script-src 'none'; connect-src 'none'; img-src blob: data:; style-src 'unsafe-inline' blob:; font-src blob:; media-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'";
const forbidden = ['script','iframe','frame','frameset','object','embed','applet','form','input','button','textarea','select','meta','base','audio','video','source','track','foreignObject','animate','animateMotion','animateTransform','set'];
function localReference(value: string, transformed: boolean): boolean {
  const v = value.trim();
  if (!v || v.startsWith('#')) return true;
  if (transformed && /^blob:/.test(v)) return true;
  if (/^data:image\/(?:png|jpeg|gif|webp);base64,[a-z0-9+/=]+$/i.test(v) && v.length <= 2*1024*1024) return true;
  return !/[\\\x00-\x20]/.test(v) && !v.startsWith('/') && !/^[a-z][a-z0-9+.-]*:/i.test(v);
}
export function secureMarkup(raw: string, mime: 'application/xhtml+xml' | 'text/html' | 'image/svg+xml', transformed = false): string {
  if (raw.length > 16*1024*1024 || /<!\s*(?:DOCTYPE|ENTITY)\b/i.test(raw.replace(/<!DOCTYPE\s+html\s*>/i,''))) throw new ReaderError('invalid_book');
  // Always parse XML first: unlike text/html, inert XML parsing cannot initiate
  // img/iframe loads. DOMPurify then handles namespaces and mutation-XSS.
  const doc = new DOMParser().parseFromString(raw, mime === 'text/html' ? 'application/xhtml+xml' : mime);
  if (doc.querySelector('parsererror') || !['html','svg'].includes(doc.documentElement.localName)) throw new ReaderError('invalid_book');
  for (const node of Array.from(doc.querySelectorAll('*'))) {
    if (forbidden.includes(node.localName)) {node.remove(); continue;}
    for (const attr of Array.from(node.attributes)) {
      const name = attr.localName.toLowerCase();
      if (name.startsWith('on') || ['srcdoc','srcset','ping','action','formaction','target','download','autofocus','http-equiv'].includes(name)) node.removeAttributeNode(attr);
      else if (['href','src','poster','data'].includes(name) && !localReference(attr.value, transformed)) node.removeAttributeNode(attr);
    }
  }
  DOMPurify.sanitize(doc.documentElement, {
    IN_PLACE: true,
    WHOLE_DOCUMENT: true, ADD_TAGS: ['link'], ADD_ATTR: ['epub:type'], FORBID_TAGS: forbidden,
    ALLOW_DATA_ATTR: false, ALLOW_UNKNOWN_PROTOCOLS: false,
    // local archive references and generated blob references only; network
    // references were removed above and CSP is the independent backstop.
    ALLOWED_URI_REGEXP: /^(?:(?:blob:|data:image\/(?:png|jpeg|gif|webp);base64,)|[^a-z]|[a-z+.-]+(?:[^a-z+.:\-]|$))/i,
  });
  if (mime === 'image/svg+xml') return new XMLSerializer().serializeToString(doc.documentElement);
  const ns = 'http://www.w3.org/1999/xhtml';
  let head = doc.querySelector('head');
  if (!head) {head = doc.createElementNS(ns, 'head'); doc.documentElement.prepend(head);}
  const meta = doc.createElementNS(ns, 'meta'); meta.setAttribute('http-equiv', 'Content-Security-Policy'); meta.setAttribute('content', BOOK_CSP);
  head.prepend(meta);
  const policy = doc.createElementNS(ns, 'meta'); policy.setAttribute('name', 'referrer'); policy.setAttribute('content', 'no-referrer'); head.prepend(policy);
  return new XMLSerializer().serializeToString(doc.documentElement);
}
export function rejectXmlEntities(raw: string): string {
  if (/<!\s*(?:DOCTYPE|ENTITY)\b/i.test(raw)) throw new ReaderError('invalid_book');
  return raw;
}
