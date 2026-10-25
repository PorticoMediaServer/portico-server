import {useEffect, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {tunerStatus, tunerUseLine} from '@core/server-admin/live-words.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button} from '../../../../kit/controls';
import {SettingsRow} from '../../../../kit/shell/SettingsSurfaces';
import type {TunerController} from './tuner-controller';
import styles from '../../../live/owner/owner.module.css';
const t = defaultI18n.t;
/** Actual source allocations, never client-estimated tuner availability. */
export function Tuners({controller}: {controller: TunerController}) {
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot), view = state.view;
  useEffect(() => {
    void controller.load();
    const refresh = () => {if (controller.isActive() && document.visibilityState !== 'hidden') void controller.load();};
    const timer = setInterval(refresh, 30000);document.addEventListener('visibilitychange', refresh);
    return () => {clearInterval(timer);document.removeEventListener('visibilitychange', refresh);};
  }, [controller]);
  if (!controller.isActive()) return null;
  return <section className={styles.section}><div className={styles.heading}><h3>{t('web.dvrSettings.tuners')}</h3>
    <Button variant="icon" size="small" icon="i-refresh" iconOnly aria-label={t('action.refresh')} disabled={state.loading} onClick={() => void controller.load()}/></div>
    {view && <p className={styles.lede}>{t('web.dvrSettings.tunersLede', {hours: view.upcomingWindowHours})}</p>}
    {!!state.error && <p role="alert" className={styles.error}>{errorMessage(state.error)}</p>}
    {!view && state.loading && <p role="status">{t('status.loadingThing', {thing: t('server.thing.tuners')})}</p>}
    {view && !view.sources.length && <p>{t('web.dvrSettings.noTuners')}</p>}
    {!!view?.sources.length && <div className={styles.sources}>{view.sources.map(source => {
      const status = tunerStatus(t, source), uses = [...source.active, ...source.upcoming.slice(0, 6)];
      return <SettingsRow key={source.sourceId} label={<>{source.sourceName} <span className={styles.badge} data-tone={status.tone}>{status.text}</span></>}>
        <div>{uses.length ? uses.map((use, index) => <p key={use.allocationId || use.recordingId || index}>
          {tunerUseLine(t, use, iso => defaultI18n.formatDate(iso, {weekday: 'short', hour: 'numeric', minute: '2-digit'}))}
        </p>) : <p>{t('web.dvrSettings.nothingScheduled')}</p>}</div>
      </SettingsRow>;
    })}</div>}
  </section>;
}
