import {useEffect, useSyncExternalStore} from 'react';
import type {HomeRow} from '@core/home.ts';
import {serverTextLabel} from '@core/presentation/server-text.ts';
import {defaultI18n} from '@i18n';
import {AmbientArt, type AmbientArtSource} from '../../kit/artwork';
import {Button} from '../../kit/controls';
import {StateView} from '../../kit/states';
import {LibraryBrowse, type LibraryBrowseBindings} from './Library';
import {homeRowReasonParts} from './home-row-reason';
import styles from './home-row-r46.module.css';

export type HomeRowBindings = Readonly<{row: HomeRow; browse: LibraryBrowseBindings; onBack: () => void; onPlay?: () => void; onShuffle?: () => void;
  /** The page's own artwork for the drawn wash; absent when backdrops are off or nothing arrived. */
  ambient?: readonly AmbientArtSource[]}>;
/** The actual row identity and reason survive every browse sort/filter and Back. */
export function HomeRowScreen({row, browse, onBack, onPlay, onShuffle, ambient}: HomeRowBindings) {
  const controller = browse.controller;
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  useEffect(() => {void controller.load();}, [controller]);
  if (state.phase === 'error') return <StateView title={defaultI18n.t('next.states.pageFailed')} body={defaultI18n.t('next.states.pageFailedBody')} primary={{id: 'retry', label: defaultI18n.t('next.states.tryAgain'), onPress: controller.retry}}/>;
  return <section className={styles.homeRow} data-home-row={row.id}>
    {ambient && ambient.length > 0 && <AmbientArt className={styles.ambient} sources={ambient} treatment="library" width="100%" height={560}/>}
    <header className={styles.header}>
      <Button variant="text" size="small" icon="i-left" className={styles.back} onClick={onBack}>{defaultI18n.t('next.home.shell.home')}</Button>
      <h1 className={styles.title}>{serverTextLabel(defaultI18n, row.titleText, row.title)}</h1>
      {row.why && <p className={styles.why}>{homeRowReasonParts(row, defaultI18n).map((part, index) => part.titleId ? <strong key={index}>{part.text}</strong> : part.text)}</p>}
      {(onPlay || onShuffle) && <div className={styles.actions}>{onPlay && <Button variant="primary" size="hero" icon="i-play" onClick={onPlay}>{defaultI18n.t('action.play')}</Button>}{onShuffle && <Button variant="glass" size="hero" icon="i-shuffle" onClick={onShuffle}>{defaultI18n.t('title.shuffle')}</Button>}</div>}
    </header>
    <LibraryBrowse {...browse} showSaveView={false}/>
  </section>;
}
