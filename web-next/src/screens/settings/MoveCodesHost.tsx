import {useEffect, useRef, useState} from 'react';
import type {AccountMoveCode} from '@core/account-move.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import type {SettingsOwnerSwitch} from './owner-switch';
import {MoveCodesSheet} from './MoveCodesSheet';
import styles from './owner-signin-switch.module.css';

const t = defaultI18n.t;

/**
 * Frame 24d's post-move host: the owner makes one sheet of codes for the exact
 * people still waiting. The waiting read and the code receipt are bounded and
 * owner-fenced on the server; this surface holds the receipt in memory, captures
 * the rendered console address, retires on close and drops everything when the
 * view, the authority or the sign-in direction changes. It issues nothing
 * itself and admits nobody.
 */
export function MoveCodesHost({source, serverName, isCurrent, onError}: {
	source: SettingsOwnerSwitch; serverName: string; isCurrent: () => boolean; onError: (error: unknown) => void;
}) {
	const [waiting, setWaiting] = useState<Readonly<{items: readonly {accountId: string; username: string}[]; truncated: boolean}>>();
	const [codes, setCodes] = useState<readonly AccountMoveCode[]>();
	const [busy, setBusy] = useState(false), [note, setNote] = useState('');
	const pending = useRef(false), alive = useRef(true), lease = useRef(0);
	useEffect(() => {alive.current = true; return () => {alive.current = false; lease.current++; setCodes(undefined);};}, []);
	const current = (ticket: number) => alive.current && isCurrent() && lease.current === ticket;
	const make = async () => {
		const ticket = lease.current;
		if (pending.current || !current(ticket)) return;
		pending.current = true; setBusy(true); setNote('');
		try {
			const list = await source.waiting();
			if (!current(ticket)) return;
			setWaiting(list);
			if (!list.items.length) {setNote(t('accounts.move.nobodyWaiting')); return;}
			const receipt = await source.codes(list.items.map(row => row.accountId));
			if (!current(ticket)) return;
			setCodes(receipt);
		} catch (error) {if (current(ticket)) onError(error);}
		finally {pending.current = false; if (alive.current) setBusy(false);}
	};
	const close = () => {lease.current++; setCodes(undefined);};
	return <>
		<div className={styles.codesRow}>
			<span>{t('accounts.move.sheetTitle')}
				<small>{waiting ? t(waiting.truncated ? 'accounts.move.waitingTruncated' : 'accounts.move.waitingCount', {count: defaultI18n.number(waiting.items.length)}) : t('accounts.move.waitingHelp')}</small>
			</span>
			<Button variant="glass" size="small" loading={busy} disabled={busy} onClick={() => void make()}>{t('accounts.move.makeCodes')}</Button>
		</div>
		{note && <p className={styles.hostNote}>{note}</p>}
		{codes && <MoveCodesSheet codes={codes} serverName={serverName} address={location.origin} isCurrent={() => alive.current && isCurrent()} onClose={close} onError={onError}/>}
	</>;
}
