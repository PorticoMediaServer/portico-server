'use strict';
// Runs the consent page's script against a small stand-in document and prints
// what a person would have been shown and what the page sent. Arguments: the
// script, the string catalogue, and the review answer (JSON).
const fs = require('fs');
const [script, catalogue, reviewJSON] = process.argv.slice(2);
const review = JSON.parse(reviewJSON);

let now = 1000000;
Date.now = () => now;
let timers = [];
globalThis.setTimeout = (fn, ms) => { timers.push({ fn, at: now + (ms || 0) }); return timers.length; };
const advance = ms => {
  now += ms;
  const due = timers.filter(t => t.at <= now);
  timers = timers.filter(t => t.at > now);
  due.forEach(t => t.fn());
};

class Node {
  constructor(tag) { this.tag = tag; this.children = []; this.own = ''; this.hidden = false; this.disabled = false; this.listeners = {}; }
  get textContent() { return this.children.length ? this.children.map(c => c.textContent).join('') : this.own; }
  set textContent(v) { this.children = []; this.own = String(v); }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = [...nodes]; this.own = ''; }
  addEventListener(type, fn) { this.listeners[type] = fn; }
  focus() { document.activeElement = this; }
  // A disabled button receives no click, from a pointer or from the keyboard.
  async click() { if (!this.disabled && this.listeners.click) await this.listeners.click(); }
}
const byId = {};
const el = id => byId[id] ?? (byId[id] = Object.assign(new Node('p'), { id }));
for (const id of ['review', 'approve', 'deny']) el(id).tag = 'button';
el('strings').own = fs.readFileSync(catalogue, 'utf8');
el('request').own = JSON.stringify({ appName: review.appName });
globalThis.document = {
  activeElement: null,
  cookie: '__Host-portico_refresh_csrf=token',
  getElementById: el,
  querySelectorAll: selector => selector === 'button' ? [el('review'), el('approve'), el('deny')] : [],
  createElement: tag => new Node(tag),
  createTextNode: text => Object.assign(new Node('#text'), { own: String(text) }),
};
globalThis.history = { replaceState() {} };
const assigned = [];
globalThis.location = { protocol: 'https:', assign: url => assigned.push(url) };
const calls = [];
globalThis.fetch = async (path, init) => {
  calls.push(path);
  const answer = path === '/v1/app-authorizations' ? review : { location: 'my-player://done?code=one' };
  return { ok: true, status: 200, headers: { get: () => null }, json: async () => answer };
};

(0, eval)(fs.readFileSync(script, 'utf8'));

(async () => {
  const out = {};
  // The person presses Continue, and the same key or click arrives again at once.
  el('review').focus();
  await el('review').click();
  out.focusAfterReview = document.activeElement && document.activeElement.id;
  out.consentText = el('consent-line').textContent;
  out.consentParts = el('consent-line').children.map(c => ({ tag: c.tag, text: c.textContent }));
  out.allowDisabledAtFirst = el('approve').disabled;
  if (document.activeElement) await document.activeElement.click();
  await el('approve').click();
  out.callsAfterRepeat = [...calls];
  advance(5000);
  out.allowDisabledLater = el('approve').disabled;
  await el('approve').click();
  out.callsAfterDeliberateAllow = [...calls];
  out.assigned = assigned;
  console.log(JSON.stringify(out));
})().catch(e => { console.error(e); process.exit(1); });
