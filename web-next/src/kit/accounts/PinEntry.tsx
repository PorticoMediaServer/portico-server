import {useEffect, useRef, useState} from 'react';
import styles from './accounts.module.css';

export type PinEntryProps = {
	value: string;
	onChange: (value: string) => void;
	/** Called once the last digit is entered: no Continue button. */
	onComplete?: (pin: string) => void;
	length?: number;
	/** A wrong PIN: the boxes shake, clear, and this line says why. */
	error?: string;
	/** Increment to shake again for a repeated error with the same text. */
	errorKey?: number;
	onForgot?: () => void;
	forgotLabel?: string;
	label: string;
	autoFocus?: boolean;
	disabled?: boolean;
};

/**
 * PIN entry, the web port of the native four-box interaction: one digit per box,
 * filled from the number pad, and submitted on the last digit rather than with a
 * Continue button. A hidden number field carries autofill (`oneTimeCode`) and the
 * keyboard, while the boxes stay decorative so a screen reader hears one labelled
 * field. A wrong PIN clears the boxes, shakes them and reads the reason.
 */
export function PinEntry({value, onChange, onComplete, length = 4, error, errorKey, onForgot, forgotLabel, label, autoFocus = true, disabled}: PinEntryProps) {
	const input = useRef<HTMLInputElement>(null);
	const [shake, setShake] = useState(0);
	useEffect(() => {
		if (!error) return;
		onChange('');
		setShake(key => key + 1);
		// The error text and its key are the whole trigger.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [error, errorKey]);
	useEffect(() => {
		if (!autoFocus || disabled) return;
		input.current?.focus();
	}, [autoFocus, disabled]);
	const set = (next: string) => {
		const digits = next.replace(/\D/g, '').slice(0, length);
		if (digits === value) return;
		onChange(digits);
		if (digits.length === length) onComplete?.(digits);
	};
	return <div className={styles.pin} data-shake={shake} role="group" aria-label={label}>
		<div className={styles.pinBoxes} aria-hidden="true">
			{Array.from({length}, (_, index) => <span key={index} className={styles.pinBox} data-filled={index < value.length || undefined}>{index < value.length ? '•' : ''}</span>)}
		</div>
		<input ref={input} className={styles.pinInput} type="text" inputMode="numeric" pattern="[0-9]*" autoComplete="one-time-code"
			aria-label={label} value={value} maxLength={length} disabled={disabled}
			onChange={event => set(event.currentTarget.value)}/>
		{error && <p className={styles.pinError} role="alert">{error}</p>}
		{onForgot && <button type="button" className={styles.pinForgot} onClick={onForgot} disabled={disabled}>{forgotLabel}</button>}
	</div>;
}
