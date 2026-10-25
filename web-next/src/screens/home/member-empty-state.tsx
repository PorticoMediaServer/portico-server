import {defaultI18n} from '@i18n';
import {StateLayout} from '../../kit/states';
import {router} from '../../router';
import {useShellWidth} from '../../shell/viewport';
import type {ProtectedLibraryScreen} from '../browse/screen-context';
import styles from './member-empty-state.module.css';

const t = defaultI18n.t;
/** No visible library is an empty shared view, never evidence of an empty server. */
export function MemberEmptyState({captured}: {captured: ProtectedLibraryScreen}) {
  const width = useShellWidth(), context = captured.context;
  const switchServer = () => {
    if (context.isCurrent()) void router.navigate({to: '/servers'}).catch(captured.binding.home.effects.reportError);
  };
  return <StateLayout className={styles.page} backdrop={{kind: 'colour', shape: 'top'}}
    contentStyle={{top: width >= 1024 ? 286 : 250}} icon="i-clock"
    title={t('next.states.gettingReady', {server: context.facts.serverName})} body={t('next.states.gettingReadyBody')}
    links={[{id: 'switch', label: t('next.states.switchServer'), icon: 'i-server', onPress: switchServer}]}/>;
}
