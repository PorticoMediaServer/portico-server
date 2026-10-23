import {StrictMode, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {SettingsPeopleClient} from '../../../packages/client-core/src/settings-people';
import {PeopleScreen, type PeopleBindings} from '../../src/screens/settings/People';
import '../../src/kit/tokens.css';
import '../../src/kit/fonts.css';
import '../../src/bootstrap.css';

// Controlled real-client authority proof, not a server/runtime or visual acceptance fixture.
const listeners = new Set<() => void>(), proof = {requests: 0, authorityChanges: 0, errors: 0, invites: 0};
let epoch = 1, live = true, release = () => {};
const page = (username: string) => ({items: [{accountId: username, username, role: 'member', revision: 1, limitsRevision: 1, disabled: false, status: 'active', hasMoveCode: false,
  allowedLibraries: [], libraryNames: [], maxStreams: 0, profileCount: 1, profileNames: ['Fixture profile'], liveState: 'idle'}], totalCount: 1, matchedCount: 1, nextCursor: '', limit: 40});
function binding(family: string, username: string, hold = false): PeopleBindings {
  const captured = epoch, current = () => live && epoch === captured;
  const client = new SettingsPeopleClient({async request<T>(_path: string, _method?: string, _body?: unknown, signal?: AbortSignal): Promise<T> {
    proof.requests++;
    if (new URLSearchParams(location.search).has('denied')) throw Object.assign(new Error('fixture_forbidden'), {status: 403});
    if (hold) await new Promise<void>((resolve, reject) => {release = resolve;signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), {once: true});});
    return page(username) as T;
  }}, current);
  return {client, serverName: 'Fixture server', libraries: [], actor: {accountId: 'fixture-owner', role: 'owner'},
    authority: {scope: {serverId: 'fixture-server', authority: 'local', accountId: 'fixture-owner', profileId: 'fixture-profile', sessionFamilyId: family}, isCurrent: current,
      subscribe(listener) {listeners.add(listener);return () => {listeners.delete(listener);};}, onAuthorityChanged() {proof.authorityChanges++;}},
    invite() {proof.invites++;}, openHome() {}, reportError() {proof.errors++;}};
}
function Fixture() {
  const [value, setValue] = useState(() => binding('family-one', 'private-first'));
  Object.assign(window, {peopleAuthority: {proof,
    revoke() {live = false;for (const listener of listeners) listener();},
    replace() {epoch++;live = true;setValue(binding('family-two', 'private-second', true));for (const listener of listeners) listener();},
    release() {release();},
  }});
  return <PeopleScreen bindings={value} />;
}
createRoot(document.getElementById('root')!).render(<StrictMode><Fixture /></StrictMode>);
