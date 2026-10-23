import type {ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {MessageId} from '@i18n';
import {Select, Button} from '../../kit/controls';
import type {ReadState} from './use-read';
import styles from './dashboard.module.css';
export const t = defaultI18n.t;
export const mediaLabel = (kind: string) => t(({movies: 'next.dashboard.movies', tv: 'next.dashboard.tv', music: 'next.dashboard.music', audiobooks: 'next.dashboard.audiobooks', live: 'next.dashboard.liveMedia', all: 'next.dashboard.allMedia'} as Record<string, MessageId>)[kind] ?? 'next.dashboard.other');
export function Range({value, onChange, values}: {value: string; onChange: (value: string) => void; values: readonly string[]}) {
  return <Select aria-label={t('next.dashboard.range')} value={value} onValueChange={onChange} options={values.map(value => ({value, label: t(`next.dashboard.${value}` as MessageId)}))} />;
}
export function Section({title, extra, actions, children, id}: {title: string; extra?: ReactNode; actions?: ReactNode; children: ReactNode; id?: string}) {
  return <section className={styles.section} id={id}><div className={styles.heading}><h3>{title}</h3>{extra}<div className={styles.filters}>{actions}</div></div>{children}</section>;
}
export function ReadFeedback({state}: {state: ReadState<unknown> & {refresh: () => void}}) {
  return state.failed ? <div className={styles.empty} role="alert">{t('next.dashboard.error')}<Button size="xs" onClick={state.refresh}>{t('next.dashboard.retry')}</Button></div> : state.loading && !state.value ? <p className={styles.empty} role="status">{t('next.dashboard.loading')}</p> : null;
}
