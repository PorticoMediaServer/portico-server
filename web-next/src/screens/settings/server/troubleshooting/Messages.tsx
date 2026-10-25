import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import {logCategories, logLevels, type LogLevel, type LogCategory} from '@core/server-administration.ts';
import {Button, Select, Switch} from '../../../../kit/controls';
import {SettingsMessagesSource} from './messages-source';
import styles from './troubleshooting.module.css';
const t = defaultI18n.t;
const levelKey = (level: LogLevel) => `web.logs.level.${level}` as MessageId;
const categoryKey = (category: LogCategory) => `web.logs.category.${category}` as MessageId;

export function MessagesPanel({source, reportError}: Readonly<{source: SettingsMessagesSource; reportError: (error: unknown) => void}>) {
  const state = useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
  const box = useRef<HTMLDivElement>(null);
  const [now, setNow] = useState(Date.now());
  useEffect(() => {void source.load();return source.pause;}, [source]);
  useEffect(() => {const timer = setInterval(() => setNow(Date.now()), 30000);return () => clearInterval(timer);}, []);
  useEffect(() => {if (state.live && box.current) box.current.scrollTop = box.current.scrollHeight;}, [state.lines, state.live]);
  const until = state.settings?.debugWindowUntil, detailed = !!until && Date.parse(until) > now;
  return <div className={styles.reports}>
    {state.settings && <section className={styles.detailWindow}>
      <div><b>{t('settings.server.detailWindow')}</b><p className={styles.help}>{detailed ? t('settings.server.detailWindow.until', {time: defaultI18n.dateTime(until!)}) : t('settings.server.detailWindow.help')}</p></div>
      <Button size="small" variant="glass" disabled={state.changing} onClick={() => {void source.detail(!detailed).catch(reportError);}}>{t(detailed ? 'settings.server.detailWindow.stop' : 'settings.server.detailWindow.start')}</Button>
    </section>}
    <section className={styles.messages}>
      <div className={styles.panelHead}><h3>{t('web.logs.messages')}</h3><span className={styles.tag}>{t('web.logs.adminVisible')}</span></div>
      <p className={styles.lede}>{t('web.logs.messagesLede')}</p>
      {state.phase === 'error' && <p className={styles.failure} role="alert"><span>{t('web.logs.readFailed')}</span><button type="button" onClick={() => {void source.load();}}>{t('next.shell.tryAgain')}</button></p>}
      <div className={styles.messageFilters}>
        <Select aria-label={t('web.logs.level')} value={state.level || 'all'} onValueChange={value => {void source.select(value === 'all' ? '' : value as LogLevel, state.category);}}
          options={[{value: 'all', label: t('web.logs.allLevels')}, ...logLevels.map(level => ({value: level, label: t('web.logs.levelAndAbove', {level: t(levelKey(level))})}))]} />
        <Select aria-label={t('feedback.category')} value={state.category || 'all'} onValueChange={value => {void source.select(state.level, value === 'all' ? '' : value as LogCategory);}}
          options={[{value: 'all', label: t('web.logs.everything')}, ...logCategories.map(category => ({value: category, label: t(categoryKey(category))}))]} />
        <label className={styles.liveChoice}><Switch checked={state.live} onCheckedChange={source.setLive} />{t('guide.live')}</label>
        {state.live ? <span className={styles.following} data-paused={state.reconnecting || undefined} role="status">{t(state.reconnecting ? 'web.logs.reconnecting' : 'web.logs.following')}</span>
          : <><span className={styles.note}>{state.readAt ? t('web.logs.updated', {time: defaultI18n.dateTime(state.readAt)}) : ''}</span><Button size="small" variant="glass" disabled={state.phase === 'loading'} onClick={() => {void source.load();}}>{t('web.logs.refresh')}</Button></>}
      </div>
      <div ref={box} className={styles.messageLog} role="log" aria-live="off" aria-busy={state.phase === 'loading'}>
        {state.lines.map(line => <div key={line.sequence} className={styles.messageLine}>
          <time dateTime={line.at}>{defaultI18n.dateTime(line.at)}</time><b data-level={line.level}>{t(levelKey(line.level))}</b><span>{line.category}</span><span>{line.message}</span>
        </div>)}
        {!state.lines.length && <p className={styles.none}>{t(state.phase === 'loading' || state.phase === 'idle' ? 'web.logs.loadingMessages' : 'web.logs.noMessages')}</p>}
      </div>
    </section>
  </div>;
}
