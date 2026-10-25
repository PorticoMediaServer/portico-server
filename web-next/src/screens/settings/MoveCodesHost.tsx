import {useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore} from 'react';
import type {AccountMove, AccountMoveCode, AccountMoveWaiting} from '@core/account-move.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import type {SettingsOwnerSwitch} from './owner-switch';
import {useMoveCodePublisher} from './move-codes-port';
import {MoveCodesSheet} from './MoveCodesSheet';
import styles from './owner-signin-switch.module.css';

const t = defaultI18n.t;
type Waiting = Readonly<{items: readonly AccountMoveWaiting[]; truncated: boolean}>;
/** One committed flight: this exact source, move revision and lifetime ticket.
 * Handlers are rendered against one flight and can only ever act on it. */
type Flight = {
	readonly id: number; readonly source: SettingsOwnerSwitch; readonly contextId: string;
	readonly move: AccountMove; readonly controller: AbortController;
	retired: boolean; pending: boolean; launched: boolean;
};
type Held<T> = Readonly<{flight: Flight; value: T}>;
type Receipt = Readonly<{flight: Flight; codes: readonly AccountMoveCode[]; targets: readonly string[]; issuer: string}>;

const retire = (flight: Flight) => {if (!flight.retired) {flight.retired = true; flight.controller.abort();}};

/**
 * Frames 24d and 24c2: the owner makes codes for the exact people still waiting,
 * one person at a time or everyone listed. Each flight is bound to one exact
 * source, move revision and abort ticket; a replaced move retires the previous
 * flight during render (before its layout cleanup) and every handler only acts
 * through the flight it was drawn with, so a retained button cannot issue a
 * successor's targets or finish a move it did not draw. The sheet's address is
 * the issuer origin proved for this server, never the page's own location.
 */
export function MoveCodesHost({source, serverName, isCurrent, onError}: {
	source: SettingsOwnerSwitch; serverName: string; isCurrent: () => boolean; onError: (error: unknown) => void;
}) {
	const move = useSyncExternalStore(source.subscribeMove, source.getMove);
	const [waiting, setWaiting] = useState<Held<Waiting>>();
	const [receipt, setReceipt] = useState<Receipt>();
	const [busy, setBusy] = useState<Held<string>>();
	const [note, setNote] = useState('');
	const sequence = useRef(0), flightRef = useRef<Flight | undefined>(undefined), liveRef = useRef(isCurrent);
	const sourceRef = useRef(source), refreshed = useRef('');
	liveRef.current = isCurrent; sourceRef.current = source;
	const contextId = source.contextId, moveId = move?.id, revision = move?.revision;
	// Install the flight for this render, retiring a replaced one immediately.
	let flight = flightRef.current;
	if (flight && (!move || flight.contextId !== contextId || flight.move.id !== moveId || flight.move.revision !== revision)) {
		retire(flight); flight = undefined; flightRef.current = undefined;
	}
	if (!flight && move) {
		flight = {id: ++sequence.current, source, contextId, move, controller: new AbortController(), retired: false, pending: false, launched: false};
		flightRef.current = flight;
	}
	const live = (target: Flight | undefined): target is Flight =>
		target !== undefined && target === flightRef.current && !target.retired && !target.controller.signal.aborted && liveRef.current();
	// One read per captured context, not per rebuilt source object.
	useEffect(() => {if (refreshed.current !== contextId) {refreshed.current = contextId; sourceRef.current.refreshMove();}}, [contextId]);
	const flightId = flight?.id;
	useLayoutEffect(() => {
		if (!flight) return;
		setWaiting(undefined); setReceipt(undefined); setBusy(undefined); setNote('');
		if (!flight.launched) {
			flight.launched = true;
			void flight.source.waiting(flight.controller.signal).then(list => {
				if (live(flight)) setWaiting({flight, value: list});
			}, () => undefined);
		}
		return () => {retire(flight);};
	}, [flightId]);
	const makeFor = async (target: Flight, ids?: readonly string[]) => {
		const list = waiting && waiting.flight === target ? waiting.value : undefined;
		if (!live(target) || target.pending || !list) return;
		const targets = ids ?? list.items.map(row => row.accountId);
		// Exactly the rendered people: no invented target, and no whole-population
		// action while the read proves it listed only the first 2000.
		if (!targets.length || targets.some(id => !list.items.some(row => row.accountId === id))) return;
		if (!ids && list.truncated) return;
		const issuer = target.source.issuerOrigin();
		if (!issuer) {setNote(t('accounts.move.noIssuer')); return;}
		target.pending = true; setBusy({flight: target, value: ids ? ids[0] ?? '' : 'all'}); setNote('');
		try {
			const codes = await target.source.codes(targets, target.controller.signal);
			if (!live(target)) return;
			// The receipt must be exactly these targets before anything is shown.
			if (codes.length !== targets.length || targets.some(id => !codes.some(code => code.accountId === id))) return;
			setReceipt({flight: target, codes, targets, issuer});
		} catch (error) {if (live(target)) onError(error);}
		finally {
			target.pending = false;
			setBusy(current => current && current.flight === target ? undefined : current);
		}
	};
	const finish = async (target: Flight) => {
		if (!live(target) || target.pending) return;
		target.pending = true; setBusy({flight: target, value: 'finish'}); setNote('');
		try {await target.source.finishMove(target.move, {isCurrent: liveRef.current, signal: target.controller.signal});}
		catch (error) {if (live(target)) onError(error);}
		finally {
			target.pending = false;
			setBusy(current => current && current.flight === target ? undefined : current);
		}
	};
	// A retained sheet may dismiss only the receipt it was drawn for.
	const close = (target: Receipt) => {setReceipt(current => current === target ? undefined : current);};
	const current = flight, list = waiting && waiting.flight === current ? waiting.value : undefined;
	const receiptShown = receipt && receipt.flight === current ? receipt : undefined;
	const held = busy && busy.flight === current ? busy.value : undefined;
	// Publish this flight's narrow row port while it stands; the People table's row
	// action then issues through this same receipt owner, never a second one.
	const publishPort = useMoveCodePublisher();
	useLayoutEffect(() => {
		if (!current) {publishPort(undefined); return;}
		const expires = new Map<string, string>();
		if (list) for (const row of list.items) if (row.expiresAt) expires.set(row.accountId, row.expiresAt);
		publishPort({expires, busy: held ?? '', make: accountId => {void makeFor(current, [accountId]);}});
		return () => publishPort(undefined);
	}, [flightId, list, held]);
	return <>
		<div className={styles.codesRow}>
			<span>{t('accounts.move.sheetTitle')}
				<small>{move ? t('accounts.settings.waiting', {waiting: move.waiting, total: move.people})
					: list ? t(list.truncated ? 'accounts.move.waitingTruncated' : 'accounts.move.waitingCount', {count: defaultI18n.number(list.items.length)})
						: t('accounts.move.waitingHelp')}</small>
			</span>
			{current && list ? <Button variant="glass" size="small" loading={held === 'all'} disabled={Boolean(held) || list.truncated}
				onClick={() => void makeFor(current)}>{t('accounts.move.makeCodes')}</Button> : null}
			{current ? <Button variant="glass" size="small" loading={held === 'finish'} disabled={Boolean(held)} onClick={() => void finish(current)}>{t('accounts.settings.finishNow')}</Button> : null}
		</div>
		{move ? <p className={styles.hostNote}>{t('accounts.settings.finishNowHelp')}</p> : null}
		{note && <p className={styles.hostNote}>{note}</p>}
		{receiptShown ? <MoveCodesSheet codes={receiptShown.codes} serverName={serverName} address={receiptShown.issuer}
			isCurrent={() => live(receiptShown.flight)} onClose={() => close(receiptShown)} onError={onError}/> : null}
	</>;
}
