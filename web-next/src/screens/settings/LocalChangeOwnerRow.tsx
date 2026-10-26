import {useLayoutEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {router} from '../../router';
import {Button} from '../../kit/controls';
import {LocalChangeOwnerDialog} from './LocalChangeOwnerDialog';
import type {LocalOwnerTransferPort, LocalOwnerTransferSource} from './local-owner-transfer';

const t = defaultI18n.t;

/** One rendered opening of the local Change owner row. The source, the exact
 * location object it was opened under and the route's liveness are captured before
 * the read; only this ticket's port may be closed by its retirement. */
type Ticket = {
  readonly source: LocalOwnerTransferSource;
  readonly location: typeof router.state.location;
  readonly isCurrent: () => boolean;
  readonly abort: AbortController;
  live: boolean;
  port?: LocalOwnerTransferPort;
};

/** auth 24a/24h: the Sign-in row and its dialog. The row owns the opening; the
 * guarded source owns the read and the transfer. */
export function LocalChangeOwnerRow({source, isCurrent, onError}: {
  source: LocalOwnerTransferSource;
  isCurrent: () => boolean;
  onError: (error: unknown) => void;
}) {
  const [ticket, setTicket] = useState<Ticket | undefined>(undefined), [port, setPort] = useState<LocalOwnerTransferPort | undefined>(undefined), [opening, setOpening] = useState(false);
  const ticketRef = useRef<Ticket | undefined>(undefined);
  const retire = (held: Ticket) => {
    held.live = false;
    held.abort.abort();
    if (ticketRef.current === held) ticketRef.current = undefined;
    const owned = held.port;
    held.port = undefined;
    owned?.close();
  };
  // A replaced source or unmount retires this row's opening synchronously.
  useLayoutEffect(() => () => {
    const held = ticketRef.current;
    if (!held) return;
    retire(held);
    setPort(undefined);
    setTicket(undefined);
    setOpening(false);
  }, [source]);
  const open = () => {
    if (ticketRef.current || opening || !isCurrent()) return;
    const held: Ticket = {source, location: router.state.location, isCurrent, abort: new AbortController(), live: true};
    ticketRef.current = held;
    setTicket(held);setPort(undefined);setOpening(true);
    const caller = {isCurrent: () => held.live && ticketRef.current === held && router.state.location === held.location &&
      held.isCurrent() && !held.abort.signal.aborted, signal: held.abort.signal};
    void (async () => {
      try {
        const opened = await source.open(caller);
        if (!held.live || ticketRef.current !== held || !caller.isCurrent()) {opened.close();return;}
        held.port = opened;
        setPort(opened);setOpening(false);
      } catch (failure) {
        if (!held.live || ticketRef.current !== held) return;
        retire(held);
        setPort(undefined);setTicket(undefined);setOpening(false);
        if (!(failure instanceof DOMException && failure.name === 'AbortError')) onError(failure);
      }
    })();
  };
  const close = () => {
    const held = ticketRef.current;
    if (held) retire(held);
    setPort(undefined);setTicket(undefined);setOpening(false);
  };
  return <>
    <Button variant="glass" size="small" loading={opening} disabled={opening} onClick={open}>
      {t('next.settingsSpec.server.signin.groups.0.rows.5.control.label')}
    </Button>
    {ticket && port ? <LocalChangeOwnerDialog open port={port} onClose={close} onError={onError} /> : null}
  </>;
}
