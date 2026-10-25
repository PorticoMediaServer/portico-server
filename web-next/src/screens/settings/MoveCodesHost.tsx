import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import type {AccountMoveCode, AccountMoveWaiting} from '@core/account-move.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import type {SettingsOwnerSwitch} from './owner-switch';
import {MoveCodesSheet} from './MoveCodesSheet';
import styles from './owner-signin-switch.module.css';

const t = defaultI18n.t;
type Waiting = Readonly<{items: readonly AccountMoveWaiting[]; truncated: boolean}>;

/**
 * Frames 24d and 24c2: the owner makes codes for the exact people still waiting,
 * one person at a time or everyone at once. The move read, the waiting read and
 * the code receipt are bounded and owner-fenced on the server; this surface holds
 * the receipt in memory, captures the rendered console address, retires on close
 * and drops everything when the view, the authority or the sign-in direction
 * changes. It issues nothing itself and admits nobody.
 */
export function MoveCodesHost({source, serverName, isCurrent, onError}: {
	source: SettingsOwnerSwitch; serverName: string; isCurrent: () => boolean; onError: (error: unknown) => void;
}) {
	const move = useSyncExternalStore(source.subscribeMove, source.getMove);
	const [waiting, setWaiting] = useState<Waiting>();
	const [codes, setCodes] = useState<readonly AccountMoveCode[]>();
	const [busy, setBusy] = useState(''), [note, setNote] = useState('');
	const pending = useRef(false), alive = useRef(true), lease = useRef(0), live = useRef(isCurrent);
	live.current = isCurrent;
	useEffect(() => {alive.current = true; return () => {alive.current = false; lease.current++; setCodes(undefined);};}, []);
	useEffect(() => {source.refreshMove();}, [source]);
	// The per-person list is read once per move revision, and only while a move runs.
	const moveId = move?.id, revision = move?.revision;
	useEffect(() => {
		if (moveId === undefined) {setWaiting(undefined); return;}
		const ticket = lease.current;
		void source.waiting().then(list => {if (alive.current && live.current() && lease.current === ticket) setWaiting(list);}, () => undefined);
	}, [moveId, revision, source]);
	const current = (ticket: number) => alive.current && isCurrent() && lease.current === ticket;
	const makeFor = async (ids?: readonly string[]) => {
		const ticket = lease.current;
		if (pending.current || !current(ticket)) return;
		pending.current = true; setBusy(ids ? ids[0] ?? 'one' : 'all'); setNote('');
		try {
			const list = ids && waiting ? waiting : await source.waiting();
			if (!current(ticket)) return;
			if (!ids) setWaiting(list);
			const targets = ids ?? list.items.map(row => row.accountId);
			if (!targets.length) {setNote(t('accounts.move.nobodyWaiting')); return;}
			const receipt = await source.codes(targets);
			if (!current(ticket)) return;
			setCodes(receipt);
		} catch (error) {if (current(ticket)) onError(error);}
		finally {pending.current = false; if (alive.current) setBusy('');}
	};
	const finish = async () => {
		if (!move || busy || !isCurrent()) return;
		setBusy('finish'); setNote('');
		try {await source.finishMove();}
		catch (error) {if (isCurrent()) onError(error);}
		finally {if (alive.current) setBusy('');}
	};
	const close = () => {lease.current++; setCodes(undefined);};
	return <>
		<div className={styles.codesRow}>
			<span>{t('accounts.move.sheetTitle')}
				<small>{move ? t('accounts.settings.waiting', {waiting: move.waiting, total: move.people})
					: waiting ? t(waiting.truncated ? 'accounts.move.waitingTruncated' : 'accounts.move.waitingCount', {count: defaultI18n.number(waiting.items.length)})
						: t('accounts.move.waitingHelp')}</small>
			</span>
			<Button variant="glass" size="small" loading={busy === 'all'} disabled={Boolean(busy)} onClick={() => void makeFor()}>{t('accounts.move.makeCodes')}</Button>
			{move ? <Button variant="glass" size="small" loading={busy === 'finish'} disabled={Boolean(busy)} onClick={() => void finish()}>{t('accounts.settings.finishNow')}</Button> : null}
		</div>
		{move ? <p className={styles.hostNote}>{t('accounts.settings.finishNowHelp')}</p> : null}
		{note && <p className={styles.hostNote}>{note}</p>}
		{move && waiting ? <ul className={styles.waitingList}>
			{waiting.items.map(row => <li key={row.accountId}>
				<span><b>{row.username}</b><small>{row.expiresAt ? `${t('accounts.move.validUntil')} ${defaultI18n.date(row.expiresAt)}` : t('accounts.move.noCodeYet')}</small></span>
				<Button variant="glass" size="small" loading={busy === row.accountId} disabled={Boolean(busy)}
					onClick={() => void makeFor([row.accountId])}>{row.expiresAt ? t('accounts.move.newCode') : t('accounts.move.makeCode')}</Button>
			</li>)}
			{waiting.truncated ? <li><small>{t('accounts.move.waitingTruncated', {count: defaultI18n.number(waiting.items.length)})}</small></li> : null}
		</ul> : null}
		{codes && <MoveCodesSheet codes={codes} serverName={serverName} address={location.origin} isCurrent={() => alive.current && isCurrent()} onClose={close} onError={onError}/>}
	</>;
}
