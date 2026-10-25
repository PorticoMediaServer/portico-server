import {useCallback, useLayoutEffect, useRef, useState} from 'react';
import {ReadingError, type ReadingClient, type ReadingReadMark, type ReadingReadMarkCommand} from '@portico/client-core';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';

type Opening = {client: ReadingClient; itemId: string; controller: AbortController; live: boolean; busy: boolean; command?: ReadingReadMarkCommand; needsRead: boolean};
type View = Readonly<{opening?: Opening; mark?: ReadingReadMark; busy?: boolean; failed?: boolean}>;
const definite = new Set(['personal_state_conflict', 'personal_needs_resolution', 'personal_needs_review', 'operation_expired', 'idempotency_key_reused', 'invalid_request', 'not_found', 'not_permitted', 'unauthorized']);

/** An explicit profile mark. Saved location and publication progress stay with
 * the existing Reader, and uncertain writes keep their original body/op ID. */
export function ReadMarkAction({client, itemId, isCurrent, reportError}: {client: ReadingClient; itemId: string; isCurrent: () => boolean; reportError: (error: unknown) => void}) {
  const inputs = useRef({client, itemId, isCurrent, reportError}); inputs.current = {client, itemId, isCurrent, reportError};
  const opening = useRef<Opening>(), [view, setView] = useState<View>({});
  const current = useCallback((held: Opening) => held.live && opening.current === held && !held.controller.signal.aborted && !held.client.signal.aborted
    && inputs.current.client === held.client && inputs.current.itemId === held.itemId && inputs.current.isCurrent(), []);
  const load = useCallback(async (held: Opening) => {
    if (!current(held) || held.busy) return;
    held.busy = true; setView(previous => ({...previous, opening: held, busy: true}));
    try {
      const mark = await held.client.readMark(held.itemId, held.controller.signal);
      if (!current(held)) return;
      held.needsRead = false; setView({opening: held, mark});
    } catch (error) {if (current(held)) {setView(previous => ({...previous, opening: held, busy: false, failed: true})); inputs.current.reportError(error);}}
    finally {held.busy = false;}
  }, [current]);
  useLayoutEffect(() => {
    const held: Opening = {client, itemId, controller: new AbortController(), live: true, busy: false, needsRead: true};
    opening.current = held; setView({opening: held});
    if (client.canMarkRead(itemId)) void load(held);
    return () => {held.live = false; held.controller.abort(); if (opening.current === held) opening.current = undefined;};
  }, [client, itemId, load]);
  const held = view.opening, mark = view.mark;
  if (!held || !current(held) || !client.canMarkRead(itemId) || !mark && !view.failed) return null;
  const press = async () => {
    if (!current(held) || held.busy) return;
    if (held.needsRead || !mark) {await load(held); return;}
    const command = held.command ?? Object.freeze({operationId: crypto.randomUUID(), expectedRevision: mark.revision, read: !mark.read});
    held.command = command; held.busy = true; setView({opening: held, mark, busy: true});
    try {
      const updated = await held.client.setReadMark(held.itemId, command, held.controller.signal);
      if (!current(held) || held.command !== command) return;
      if (updated.viewerFence !== mark.viewerFence || updated.revision < mark.revision) throw new ReadingError('source_changed');
      held.command = undefined; setView({opening: held, mark: updated});
    } catch (error) {
      if (current(held) && held.command === command) {
        if (error instanceof ReadingError && definite.has(error.code)) {held.command = undefined; held.needsRead = true;}
        setView({opening: held, mark, failed: true}); inputs.current.reportError(error);
      }
    } finally {held.busy = false;}
  };
  const label = defaultI18n.t(view.failed ? 'next.states.tryAgain' : mark?.read ? 'next.reader.markUnread' : 'next.reader.markRead');
  return <Button variant="glass" icon="i-check" iconOnly aria-label={label} title={label} aria-pressed={mark?.read} disabled={view.busy} onClick={() => {void press();}}/>;
}
