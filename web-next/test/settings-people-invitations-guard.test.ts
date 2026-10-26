import test from 'node:test';
import assert from 'node:assert/strict';
import type {ServerInvitation} from '@core/direct-account.ts';
import {
  InvitationAdmissions, invitationLease, invitationRequest, sameOpening,
  type InvitationPage,
} from '../src/screens/settings/people-invitation-admission.ts';

const component = Object.freeze({});
const at = '2026-10-22T15:41:00.000Z';
function invitation(id: string, revision = 1, state: ServerInvitation['state'] = 'pending'): ServerInvitation {
  return {id, revision, state, role: 'member', allowedLibraries: [], createdAt: at, expiresAt: '2026-10-29T15:41:00.000Z'};
}
function page(items: readonly ServerInvitation[]): InvitationPage {
  return {items, nextCursor: '', matchedCount: items.length, totalCount: items.length};
}
function screen(authority: () => boolean = () => true) {
  const guard = new InvitationAdmissions(authority);
  const request = invitationRequest(component, Object.freeze({query: 'first'}));
  guard.activate(component); guard.publishRequest(request);
  const items = [invitation('inv-1'), invitation('inv-2')];
  const lease = invitationLease(request, page(items));
  guard.publishLease(lease);
  return {guard, request, lease, items};
}

test('cancel A and reopen B on the same page: only B may be confirmed', () => {
  const {guard, lease, items} = screen();
  const a = guard.open(lease, items[0], 'operation-a')!;
  assert.equal(guard.isOpen(a), true);
  guard.close(a);                                   // synchronous, before any commit
  const b = guard.open(lease, items[0], 'operation-b')!;
  assert.equal(guard.claim(a), undefined);          // a retained Confirm A cannot act
  assert.equal(guard.claim(b), b);
  assert.equal(sameOpening(a, b), false);
});

test('an immediate cancel retires the opening before React commits', () => {
  const {guard, lease, items} = screen();
  const a = guard.open(lease, items[0], 'operation-a')!;
  guard.close(a);                                   // React state has not committed yet
  assert.equal(guard.isOpen(a), false);
  assert.equal(guard.claim(a), undefined);
});

test('an old settlement cannot clear a successor claim', () => {
  const {guard, lease, items} = screen();
  const a = guard.open(lease, items[0], 'operation-a')!;
  assert.equal(guard.claim(a), a);
  guard.release(a);                                 // the old attempt settled
  const b = guard.open(lease, items[0], 'operation-b')!;
  assert.equal(guard.claim(b), b);
  guard.release(a);                                 // a late settlement for A
  assert.equal(guard.settled(b), true);
  assert.equal(guard.commandCurrent(b), true);
});

test('a new request retires the old page lease at commit, before the page is cleared', () => {
  const {guard, request, lease, items} = screen();
  const next = invitationRequest(component, Object.freeze({query: 'second'}));
  guard.publishRequest(next);                       // the page has not been cleared yet
  assert.equal(guard.open(lease, items[0], 'operation-a'), undefined);
  assert.equal(guard.requestRead(request, ''), undefined); // reads carry the request, not the page
});

test('an old read cannot paint, release or clear the successor read', () => {
  const {guard, request} = screen();
  const first = guard.requestRead(request, '')!;
  assert.equal(guard.requestRead(request, ''), undefined); // one read per request and cursor
  const next = invitationRequest(component, Object.freeze({query: 'second'}));
  guard.publishRequest(next);
  const second = guard.requestRead(next, '')!;
  assert.equal(first.signal.aborted, true);         // superseded before it could paint
  assert.equal(guard.readCurrent(first), false);
  guard.releaseRead(first);                         // the old finally
  assert.equal(guard.readCurrent(second), true);
  guard.releaseRead(second);
  assert.equal(guard.readCurrent(second), false);
});

test('a read carrying an old query cannot start after its request retires', () => {
  const {guard, request} = screen();
  const next = invitationRequest(component, Object.freeze({query: 'second'}));
  guard.publishRequest(next);
  assert.equal(guard.requestRead(request, ''), undefined);
  assert.ok(guard.requestRead(next, ''));
});

test('a replacement request retires the command before it can settle', () => {
  const {guard, lease, items} = screen();
  const a = guard.open(lease, items[0], 'operation-a')!;
  assert.equal(guard.claim(a), a);
  const next = invitationRequest(component, Object.freeze({query: 'second'}));
  const nextItems = [invitation('inv-9')], nextLease = invitationLease(next, page(nextItems));
  guard.publishRequest(next); guard.publishLease(nextLease);
  assert.equal(guard.commandCurrent(a), false);     // no note or refresh on the new query
  guard.release(a);                                 // the old completion releases only itself
  const b = guard.open(nextLease, nextItems[0], 'operation-b')!;
  assert.equal(guard.claim(b), b);
  guard.release(a);                                 // any repeated old settlement
  assert.equal(guard.settled(b), true);
  assert.equal(guard.commandCurrent(b), true);
});

test('a replay renews the activation without reviving the old token', () => {
  const {guard, request, lease, items} = screen();
  const first = guard.activate(component);
  const a = guard.open(lease, items[0], 'operation-a')!;
  guard.retire(first);                              // StrictMode replay cleanup
  assert.equal(guard.live(), false);
  guard.activate(component);                        // replay setup: renewable, never dead
  assert.equal(guard.live(), true);
  guard.publishRequest(request); guard.publishLease(lease);
  assert.equal(guard.claim(a), undefined);
  const b = guard.open(lease, items[0], 'operation-b')!;
  assert.equal(guard.claim(b), b);
});

test('one retry keeps its operation, a reopened confirmation does not', () => {
  const {guard, lease, items} = screen();
  const a = guard.open(lease, items[0], 'operation-a')!;
  assert.equal(guard.claim(a), a);
  guard.release(a);                                 // uncertain attempt, dialog stays open
  assert.equal(guard.claim(a), a);                  // retry repeats the same request
  guard.close(a);
  const b = guard.open(lease, items[0], 'operation-b')!;
  assert.equal(guard.claim(b), b);
  assert.notEqual(b.operationId, a.operationId);
});

test('a page replacement retires a copy claim without clearing its successor', () => {
  const {guard, lease, items} = screen();
  const first = guard.copy(lease, items[0])!;
  assert.equal(guard.copy(lease, items[0]), undefined); // one copy at a time
  const next = invitationRequest(component, Object.freeze({query: 'second'}));
  const items2 = [invitation('inv-1', 2)], nextLease = invitationLease(next, page(items2));
  guard.publishRequest(next); guard.publishLease(nextLease);
  assert.equal(guard.copyCurrent(first), false);
  guard.releaseCopy(first);                          // the old finally
  const second = guard.copy(nextLease, items2[0])!;
  assert.equal(guard.copyCurrent(second), true);
  guard.releaseCopy(second);
});

test('the exact resident row, not a same-id replacement, may be commanded', () => {
  const {guard, request, items} = screen();
  const replaced = invitationLease(request, page([invitation('inv-1', 2)]));
  guard.publishLease(replaced);
  assert.equal(guard.open(replaced, items[0], 'operation-a'), undefined); // the old object
  const opening = guard.open(replaced, replaced.page!.items[0], 'operation-b')!;
  assert.equal(guard.claim(opening), opening);
});
