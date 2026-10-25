import {useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore} from 'react';
import type {AccountMove, AccountMoveCode, AccountMoveWaiting} from '@core/account-move.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import type {SettingsOwnerSwitch} from './owner-switch';
import {MoveCodesSheet} from './MoveCodesSheet';
import styles from './owner-signin-switch.module.css';

const t = defaultI18n.t;
type Waiting = Readonly<{items: readonly AccountMoveWaiting[]; truncated: boolean}>;
type Receipt = Readonly<{id: number; codes: readonly AccountMoveCode[]; targets: readonly string[]; issuer: string}>;
/** One opening: this source, this exact move revision and its own abort ticket. */
type Opening = Readonly<{id: number; move: AccountMove; controller: AbortController}>;

/**
 * Frames 24d and 24c2: the owner makes codes for the exact people still waiting,
 * one person at a time or everyone listed at once. Each opening is bound to one
 * source and one exact move revision; a replaced move retires the previous list,
 * operation and receipt synchronously, and every read, issue and finish carries
 * that opening's own signal, so an old callback can never touch its successor.
 * The server re-proves the owner; the sheet's address is the proven issuer origin
 * captured here, never the page's own location. Nothing is admitted.
 */
export function MoveCodesHost({source, serverName, isCurrent, onError}: {
	source: SettingsOwnerSwitch; serverName: string; isCurrent: () => boolean; onError: (error: unknown) => void;
}) {
	const move = useSyncExternalStore(source.subscribeMove, source.getMove);
	const [epoch, setEpoch] = useState(0);
	const [waiting, setWaiting] = useState<Waiting>();
	const [receipt, setReceipt] = useState<Receipt>();
	const [busy, setBusy] = useState(''), [note, setNote] = useState('');
	const opening = useRef<Opening | undefined>(undefined), moveRef = useRef(move), liveRef = useRef(isCurrent);
	const sourceRef = useRef(source), refreshed = useRef(''), pending = useRef(false);
	moveRef.current = move; liveRef.current = isCurrent; sourceRef.current = source;
	const contextId = source.contextId;
	const active = (opening_: Opening | undefined): opening_ is Opening =>
		opening_ !== undefined && opening.current === opening_ && !opening_.controller.signal.aborted && liveRef.current();
	// One read per captured context, not per rebuilt source object.
	useEffect(() => {if (refreshed.current !== contextId) {refreshed.current = contextId; sourceRef.current.refreshMove();}}, [contextId]);
	const moveId = move?.id, revision = move?.revision;
	// A replaced source or move retires the previous opening before this one paints.
	useLayoutEffect(() => {
		opening.current?.controller.abort();
		const current = moveRef.current;
		const next = current ? {id: (opening.current?.id ?? 0) + 1, move: current, controller: new AbortController()} : undefined;
		opening.current = next; pending.current = false;
		setEpoch(next?.id ?? 0); setWaiting(undefined); setReceipt(undefined); setNote(''); setBusy('');
		if (!next) return;
		void sourceRef.current.waiting(next.controller.signal).then(list => {
			if (active(next)) setWaiting(list);
		}, () => undefined);
		return () => {next.controller.abort();};
	}, [contextId, moveId, revision]);
	const makeFor = async (ids?: readonly string[]) => {
		const current = opening.current, list = waiting;
		if (!active(current) || pending.current || !list) return;
		const targets = ids ?? list.items.map(row => row.accountId);
		// Exactly the rendered people: no invented target, and no whole-population
		// action while the read proves it listed only the first 2000.
		if (!targets.length || targets.some(id => !list.items.some(row => row.accountId === id))) return;
		if (!ids && list.truncated) return;
		const issuer = sourceRef.current.issuerOrigin();
		if (!issuer) {setNote(t('accounts.move.noIssuer')); return;}
		pending.current = true; setBusy(ids ? ids[0] ?? '' : 'all'); setNote('');
		try {
			const codes = await sourceRef.current.codes(targets, current.controller.signal);
			if (!active(current)) return;
			// The receipt must be exactly these targets before anything is shown.
			if (codes.length !== targets.length || targets.some(id => !codes.some(code => code.accountId === id))) return;
			setReceipt({id: current.id, codes, targets, issuer});
		} catch (error) {if (active(current)) onError(error);}
		finally {pending.current = false; if (active(current)) setBusy('');}
	};
	const finish = async () => {
		const current = opening.current;
		if (!active(current) || pending.current) return;
		pending.current = true; setBusy('finish'); setNote('');
		try {await sourceRef.current.finishMove(current.move, {isCurrent: liveRef.current, signal: current.controller.signal});}
		catch (error) {if (active(current)) onError(error);}
		finally {pending.current = false; if (active(current)) setBusy('');}
	};
	// Closing the sheet drops only its receipt; the move opening stays usable.
	const close = () => {setReceipt(undefined);};
	const currentReceipt = receipt && opening.current?.id === receipt.id && receipt.id === epoch ? receipt : undefined;
	const shown = move ? waiting : undefined;
	return <>
		<div className={styles.codesRow}>
			<span>{t('accounts.move.sheetTitle')}
				<small>{move ? t('accounts.settings.waiting', {waiting: move.waiting, total: move.people})
					: shown ? t(shown.truncated ? 'accounts.move.waitingTruncated' : 'accounts.move.waitingCount', {count: defaultI18n.number(shown.items.length)})
						: t('accounts.move.waitingHelp')}</small>
			</span>
			{move && shown ? <Button variant="glass" size="small" loading={busy === 'all'} disabled={Boolean(busy) || shown.truncated}
				onClick={() => void makeFor()}>{t('accounts.move.makeCodes')}</Button> : null}
			{move ? <Button variant="glass" size="small" loading={busy === 'finish'} disabled={Boolean(busy)} onClick={() => void finish()}>{t('accounts.settings.finishNow')}</Button> : null}
		</div>
		{move ? <p className={styles.hostNote}>{t('accounts.settings.finishNowHelp')}</p> : null}
		{note && <p className={styles.hostNote}>{note}</p>}
		{move && shown ? <ul className={styles.waitingList}>
			{shown.items.map(row => <li key={row.accountId}>
				<span><b>{row.username}</b><small>{row.expiresAt ? `${t('accounts.move.validUntil')} ${defaultI18n.date(row.expiresAt)}` : t('accounts.move.noCodeYet')}</small></span>
				<Button variant="glass" size="small" loading={busy === row.accountId} disabled={Boolean(busy)}
					onClick={() => void makeFor([row.accountId])}>{row.expiresAt ? t('accounts.move.newCode') : t('accounts.move.makeCode')}</Button>
			</li>)}
			{shown.truncated ? <li><small>{t('accounts.move.waitingTruncated', {count: defaultI18n.number(shown.items.length)})}</small></li> : null}
		</ul> : null}
		{currentReceipt ? <MoveCodesSheet codes={currentReceipt.codes} serverName={serverName} address={currentReceipt.issuer}
			isCurrent={() => liveRef.current() && opening.current?.id === currentReceipt.id} onClose={close} onError={onError}/> : null}
	</>;
}
