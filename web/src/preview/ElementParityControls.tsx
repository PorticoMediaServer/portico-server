import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from 'react';

function GoogleMark() {
  return <svg aria-hidden="true" viewBox="0 0 24 24"><path fill="#4285F4" d="M21.6 12.23c0-.71-.06-1.4-.18-2.07H12v3.92h5.38a4.6 4.6 0 0 1-2 3.02v2.54h3.24c1.9-1.75 2.98-4.33 2.98-7.41Z"/><path fill="#34A853" d="M12 22c2.7 0 4.98-.9 6.63-2.36l-3.24-2.54c-.9.6-2.05.96-3.39.96-2.61 0-4.82-1.76-5.61-4.13H3.04v2.62A10 10 0 0 0 12 22Z"/><path fill="#FBBC05" d="M6.39 13.93A6 6 0 0 1 6.08 12c0-.67.11-1.33.31-1.93V7.45H3.04A10 10 0 0 0 2 12c0 1.64.39 3.19 1.04 4.55l3.35-2.62Z"/><path fill="#EA4335" d="M12 5.94c1.47 0 2.79.5 3.83 1.5l2.87-2.87A9.63 9.63 0 0 0 12 2a10 10 0 0 0-8.96 5.45l3.35 2.62C7.18 7.7 9.39 5.94 12 5.94Z"/></svg>;
}

function AppleMark() {
  return <svg aria-hidden="true" viewBox="0 0 24 24"><path fill="currentColor" d="M16.77 12.53c-.02-2.24 1.83-3.33 1.91-3.38a4.08 4.08 0 0 0-3.21-1.74c-1.35-.14-2.66.81-3.35.81-.7 0-1.76-.8-2.9-.78a4.26 4.26 0 0 0-3.58 2.18c-1.55 2.68-.4 6.62 1.09 8.79.75 1.06 1.62 2.25 2.76 2.21 1.12-.05 1.54-.71 2.89-.71 1.34 0 1.73.71 2.9.68 1.2-.02 1.96-1.06 2.68-2.13a8.75 8.75 0 0 0 1.23-2.49 3.86 3.86 0 0 1-2.42-3.44Zm-2.18-6.55a3.9 3.9 0 0 0 .9-2.8 4 4 0 0 0-2.6 1.33 3.73 3.73 0 0 0-.92 2.7 3.3 3.3 0 0 0 2.62-1.23Z"/></svg>;
}

export function ProviderAction({ provider, children, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { provider: 'google' | 'apple'; children: ReactNode }) {
  return <button className={`parity-provider-action ${provider}`} type="button" {...props}>{provider === 'google' ? <GoogleMark /> : <AppleMark />}<span>{children}</span></button>;
}

export function PorticoAction({ children, className = '', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { children: ReactNode }) {
  return <button className={`parity-portico-action ${className}`.trim()} type="button" {...props}>{children}</button>;
}

export function PorticoField({ label, state, className = '', ...props }: InputHTMLAttributes<HTMLInputElement> & { label: string; state?: 'hover' | 'focus' | 'invalid' }) {
  return <label className="parity-field"><span>{label}</span><input className={`${state ? `is-${state}` : ''} ${className}`.trim()} aria-invalid={state === 'invalid' ? true : undefined} {...props} /></label>;
}
