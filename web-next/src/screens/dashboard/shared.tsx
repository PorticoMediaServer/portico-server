import {useEffect, useState, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {MessageId} from '@i18n';
import {Select} from '../../kit/controls';
import {Skeleton, StateLayout} from '../../kit/states';
import type {ReadState} from './use-read';
import styles from './dashboard.module.css';
export const t = defaultI18n.t;
export const mediaLabel = (kind: string) => t(({movies: 'next.dashboard.movies', tv: 'next.dashboard.tv', music: 'next.dashboard.music', audiobooks: 'next.dashboard.audiobooks', live: 'next.dashboard.liveMedia', all: 'next.dashboard.allMedia'} as Record<string, MessageId>)[kind] ?? 'next.dashboard.other');
export function Range({value, onChange, values, load = false}: {load?: boolean; value: string; onChange: (value: string) => void; values: readonly string[]}) {
  return <Select aria-label={t('next.dashboard.range')} value={value} onValueChange={onChange} options={values.map(value => ({value, label: t(`next.dashboard.${load && ['7d', '30d', '90d'].includes(value) ? 'load.' : ''}${value}` as MessageId)}))} />;
}
export function Section({title, extra, actions, children, id}: {title: string; extra?: ReactNode; actions?: ReactNode; children: ReactNode; id?: string}) {
  return <section className={styles.section} id={id}><div className={styles.heading}><h3>{title}</h3>{extra}<div className={styles.filters}>{actions}</div></div>{children}</section>;
}
export function ReadFeedback({state, openHome, height = 156}: {openHome: () => void; height?: number; state: ReadState<unknown> & {refresh: () => void}}) {
  const pending = state.loading && !state.value;
  const [shapes, showShapes] = useState(false);
  useEffect(() => {
    if (!pending) {showShapes(false); return;}
    const timer = setTimeout(() => showShapes(true), 300);
    return () => clearTimeout(timer);
  }, [pending]);
  return state.failed ? <div role="alert"><StateLayout className={styles.pageFailure} title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} icon="i-alert" tone="warning" secondary={{id: 'home', label: t('next.states.goHome'), onPress: openHome}} primary={{id: 'retry', label: t('next.dashboard.retry'), icon: 'i-refresh', onPress: state.refresh}} /></div> : pending && shapes ? <div aria-busy="true"><Skeleton height={height} sheen /></div> : null;
}
