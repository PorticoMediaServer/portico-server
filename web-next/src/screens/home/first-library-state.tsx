import {useEffect, useMemo, useState} from 'react';
import {defaultI18n} from '@i18n';
import {LibraryManagementService, type ManagedLibrary} from '@core/library-management.ts';
import {Icon, type IconName} from '../../kit/Icon';
import {StateLayout} from '../../kit/states';
import {router} from '../../router';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {screenLifetime} from '../detail/context';
import {AddLibrary} from '../settings/libraries/AddLibrary';
import {useShellWidth} from '../../shell/viewport';
import styles from './first-library-state.module.css';

const t = defaultI18n.t;
type Kind = ManagedLibrary['kind'];
const kinds = [
  {id: 'movie', icon: 'i-film', title: 'next.states.kindMovie', hint: 'next.states.kindMovieHint'},
  {id: 'tv', icon: 'i-tvlib', title: 'next.states.kindTV', hint: 'next.states.kindTVHint'},
  {id: 'anime', icon: 'i-category', title: 'next.states.kindAnime', hint: 'next.states.kindAnimeHint'},
  {id: 'music', icon: 'i-music', title: 'next.states.kindMusic', hint: 'next.states.kindMusicHint'},
  {id: 'audiobook', icon: 'i-book', title: 'next.states.kindAudiobook', hint: 'next.states.kindAudiobookHint'},
] as const;

/** states#14: the confirmed owner's empty server, using the real library form. */
export function FirstLibraryState({captured}: {captured: ProtectedLibraryScreen}) {
  const context = useLibraryScreenContext(captured), width = useShellWidth();
  const resources = useMemo(() => {
    const current = () => context.isCurrent() && captured.context.session().viewer.role === 'owner'
      && captured.context.facts.identity.primaryProfile === true && captured.context.facts.identity.capabilities?.manageServer === true;
    const life = screenLifetime({...context, isCurrent: current});
    const service = new LibraryManagementService({api: life.api, scope: context.scope});
    return {life, service};
  }, [context, captured.context]);
  useEffect(() => () => {resources.life.dispose(); resources.service.dispose();}, [resources]);
  const [kind, setKind] = useState<Kind>();
  const add = (value: Kind) => {if (resources.life.current()) setKind(value);};
  const setupLive = () => {
    if (resources.life.current()) void router.navigate({to: '/settings', search: {scope: 'server', section: 'live'}}).catch(context.effects.reportError);
  };
  return <>
    <StateLayout className={styles.page} backdrop={{kind: 'colour', shape: 'side'}} contentStyle={{top: width >= 1024 ? 330 : 200}}
      eyebrow={<><span className={styles.ready}/>{t('next.states.ready', {server: captured.context.facts.serverName})}</>}
      title={t('next.states.firstLibrary')} body={t('next.states.firstLibraryBody')}
      primary={{id: 'add', label: t('next.states.addLibrary'), icon: 'i-plus', onPress: () => add('movie')}}
      secondary={{id: 'live', label: t('next.states.setupLive'), icon: 'i-live', onPress: setupLive}}
      after={<section className={styles.kinds} aria-labelledby="first-library-kinds">
        <h2 id="first-library-kinds">{t('next.states.libraryKinds')}</h2>
        <div className={styles.grid}>{kinds.map(value => <button key={value.id} type="button" className={styles.kind} data-kind={value.id} onClick={() => add(value.id)}>
          <Icon name={value.icon satisfies IconName} size={26}/><b>{t(value.title)}</b><small>{t(value.hint)}</small>
        </button>)}</div>
      </section>}/>
    {kind && <AddLibrary key={kind} initialKind={kind} open onOpenChange={open => {if (!open) setKind(undefined);}}
      api={resources.life.api} service={resources.service} onError={context.effects.reportError} serverName={captured.context.facts.serverName}/>}
  </>;
}
