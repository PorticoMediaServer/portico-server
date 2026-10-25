import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {Children, isValidElement, type ReactNode} from 'react';
import {transformTsx} from './transform-tsx.mjs';
// Real dialog functions and their actual footer/close handlers; a direct
// replacement render happens before effects clean up the old controller.
const bridge = {refs: [] as {current: unknown}[], cursor: 0};
(globalThis as any).__ownerDialogHooks = bridge;
const hooks = 'data:text/javascript,' + encodeURIComponent(`const b=globalThis.__ownerDialogHooks;export const useRef=v=>b.refs[b.cursor++]??={current:v};export const useEffect=()=>{};export const useSyncExternalStore=(_s,get)=>get();`);
const loader = registerHooks({load(url, context, next) {
  const module = (source: string) => ({format: 'module' as const, source, shortCircuit: true});
  if (url.endsWith('.module.css')) return module('export default new Proxy({}, {get:(_,k)=>String(k)});');
  if (url.endsWith('/kit/controls/index.ts')) return module("export const Button='button',TextField='input',Segmented='segmented',Select='select';");
  if (url.endsWith('/kit/overlays/index.ts')) return module("export const Dialog='dialog';");
  if (url.endsWith('.tsx')) return module(transformTsx(url).replace(/from (['"])react\1/g, `from ${JSON.stringify(hooks)}`));
  return next(url, context);
}});
const {RemoteSourceDialog} = await import('../src/screens/live/owner/RemoteSourceDialog.tsx');
const {UploadSourceDialog} = await import('../src/screens/live/owner/UploadSourceDialog.tsx');
function elements(tree: ReactNode): any[] {const result: any[] = [];Children.forEach(tree, child => {if (isValidElement<any>(child)) {result.push(child);result.push(...elements(child.props.children));}});return result;}
for (const [name, component] of [['remote', RemoteSourceDialog], ['upload', UploadSourceDialog]] as const) {
  test(`${name} stale saved/close handlers cannot affect replacement dialog`, async () => {
    bridge.refs = [];bridge.cursor = 0;let finish!: (value: boolean) => void, saved = 0, closed = 0;
    const snapshot = {draft: {name: 'Stored', locator: 'https://provider.example', playlist: 'private', kind: 'm3u', guideUrl: '', viewerAccess: 'owner-only', refreshMs: 21600000, tunerCount: 0}, preview: {preview: {channels: 1, programmes: 0, names: [], diagnostics: {skippedChannels: 0, skippedProgrammes: 0, adjustedProgrammes: 0}}, channels: 1, programmes: 0}, busy: false, reading: false};
    const fake = () => ({getSnapshot: () => snapshot, subscribe: () => () => {}, isActive: () => true, dispose() {}, save: () => new Promise<boolean>(done => {finish = done;})});
    const a = fake(), b = fake();
    const props = (controller: unknown) => ({controller, close: () => {closed++;}, saved: () => {saved++;}}) as any;
    const first = component(props(a)) as any;
    const buttons = elements(first.props.footer).filter(element => element.type === 'button');
    buttons.at(-1).props.onClick();
    bridge.cursor = 0;component(props(b));
    first.props.onOpenChange(false);buttons[0].props.onClick();finish(true);await Promise.resolve();await Promise.resolve();
    assert.equal(saved, 0);assert.equal(closed, 0);
  });
}
test.after(() => {loader.deregister();delete (globalThis as any).__ownerDialogHooks;});
