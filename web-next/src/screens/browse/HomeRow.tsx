import {useEffect, useSyncExternalStore} from 'react';
import type {HomeRow} from '@core/home.ts';
import {serverTextLabel} from '@core/presentation/server-text.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';
import {StateView} from '../../kit/states';
import {LibraryBrowse, type LibraryBindings} from './Library';
import styles from './browse.module.css';

export type HomeRowBindings = Readonly<{row: HomeRow; browse: LibraryBindings; onBack: () => void; onPlay: () => void; onShuffle: () => void}>;
/** The actual row identity and reason survive every browse sort/filter and Back. */
export function HomeRowScreen({row, browse, onBack, onPlay, onShuffle}: HomeRowBindings) {
  const controller = browse.controller;
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  useEffect(() => {void controller.load();}, [controller]);
  if (state.phase === 'error') return <StateView title={defaultI18n.t('next.states.pageFailed')} body={defaultI18n.t('next.states.pageFailedBody')} primary={{id: 'retry', label: defaultI18n.t('next.states.tryAgain'), onPress: controller.retry}}/>;
  return <section className={styles.homeRow}>
    <header className={styles.homeRowHeader}><Button variant="text" size="small" icon="i-left" onClick={onBack}>{defaultI18n.t('next.home.shell.home')}</Button>
      <h1>{serverTextLabel(defaultI18n, row.titleText, row.title)}</h1>
      {row.why && <p>{serverTextLabel(defaultI18n, row.why)}</p>}
      <div className={styles.homeRowActions}><Button variant="primary" size="hero" icon="i-play" onClick={onPlay}>{defaultI18n.t('action.play')}</Button><Button variant="glass" size="hero" icon="i-shuffle" onClick={onShuffle}>{defaultI18n.t('title.shuffle')}</Button></div>
    </header>
    <LibraryBrowse {...browse}/>
  </section>;
}
