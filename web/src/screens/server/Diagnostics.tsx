import {currentI18n} from '../../app/i18n';
import {useEffect, useRef, useState} from 'react';
import {readEventStream} from '@core/index.ts';
import {retryAfterSeconds} from '../../bridge/protectedArtwork';
import {logCategories, logLevels, parseLogEvent, parseLogPage, type LogCategory, type LogLevel, type LogRecord, type LogSettings} from '@core/admin/index.ts';
import {setDetailWindow, type LogsDraft} from '@core/server-admin/server-forms.ts';
import {useServerDeps, useServerForm} from '../settings/ServerForms';
import {useAction, useRead} from '../../admin/console';
import {CAPABILITY_BEHIND_PRESENCE, CAPABILITY_ROWS, capabilityReasonId, parseCapabilities, type ServerCapability} from '../../admin/capabilities';
import {useSession} from '../../app/session';
import {Badge, Button, Freshness, Notice, Select, SettingsGroup, SettingsRow, Status, Switch, Text} from '../../ui';

const levelTone: Record<LogLevel, string> = {error: 'var(--status-danger)', warn: 'var(--status-warning)', info: 'var(--text-secondary)', debug: 'var(--text-tertiary)'};
const KEEP = 500;

/** BE-API-15: the reconnect resumes from the retained SSE id. Ids are opaque
 * (`recorderNonce:sequence`) and go back verbatim as Last-Event-ID. */
export const logTailHeaders = (token: string, lastId: string): Record<string, string> => ({
  Authorization: 'Bearer ' + token, Accept: 'text/event-stream', ...(lastId ? {'Last-Event-ID': lastId} : {}),
});
/** BE-API-15: the server's Retry-After wins on a 429; otherwise exponential backoff. */
export const logTailBackoffMs = (attempt: number): number => 1000 * 2 ** Math.min(Math.max(1, attempt), 6);
export function logTailRetryDelayMs(status: number | undefined, retryAfterMs: number | undefined, attempt: number): number {
  if (status === 429 && retryAfterMs !== undefined) return retryAfterMs;
  return logTailBackoffMs(attempt) * (1 + Math.random() / 2);
}
export type LogTailFrame = {type: 'reset'} | {type: 'record'; record: LogRecord};
/** BE-API-15: a reset frame drops the earlier log window and later frames rebuild it; records
 * still dedupe by sequence within a bounded window. */
export function applyLogTailFrame(lines: readonly LogRecord[], frame: LogTailFrame): LogRecord[] {
  if (frame.type === 'reset') return [];
  if (lines.some(x => x.sequence === frame.record.sequence)) return [...lines];
  return [...lines.slice(-(KEEP - 1)), frame.record];
}
/** BE-API-15: the client reconnects when the 30-minute stream ends normally,
 * when the connection drops, and on 429 (with backoff). It stops only when
 * the view unmounts or live is toggled off (aborted). */
export function logTailShouldReconnect(aborted: boolean): boolean {
  return !aborted;
}

/** The server's own message log, with a live tail. The tail is a convenience: if it drops the
 * lines already shown stay, and it reconnects with a growing delay. */
export function MessageLogGroup() {
  const {api, session} = useSession();
  const t = currentI18n().t;
  const token = session?.accessToken ?? '';
  // Read the newest token at each (re)connect: rotation must not tear down a healthy tail.
  const tokenRef = useRef(token);
  tokenRef.current = token;
  const signedIn = !!token;
  const [level, setLevel] = useState<LogLevel | ''>('');
  const [category, setCategory] = useState<LogCategory | ''>('');
  const [live, setLive] = useState(false);
  const [lines, setLines] = useState<LogRecord[]>([]);
  const [tailError, setTailError] = useState('');
  const query = `${level ? `&level=${level}` : ''}${category ? `&category=${category}` : ''}`;
  const read = useRead(async () => parseLogPage(await api.request('/v1/admin/logs?limit=200' + query)), [api, query]);
  const action = useAction();
  useEffect(() => { if (read.data) setLines([...read.data.items].reverse()); }, [read.data]);
  const filter = useRef({level, category});
  filter.current = {level, category};
  // BE-API-15: the latest SSE id, retained across reconnects for Last-Event-ID resume.
  const lastId = useRef('');
  useEffect(() => {
    if (!live || !signedIn) return;
    const controller = new AbortController();
    let attempt = 0, timer: ReturnType<typeof setTimeout> | undefined;
    const order: LogLevel[] = ['error', 'warn', 'info', 'debug'];
    const connect = async () => {
      try {
        // The retained id resumes the ring buffer; a reset frame (id too old) restarts the window.
        const response = await api.routeFetch(api.baseUrl + '/v1/admin/logs/events', {headers: logTailHeaders(tokenRef.current, lastId.current), signal: controller.signal});
        if (!response.ok) {
          const asked = response.status === 429 ? retryAfterSeconds(response.headers.get('Retry-After')) : undefined;
          void response.body?.cancel().catch(() => {});
          throw Object.assign(new Error('The event stream was refused.'), {status: response.status, ...(asked === undefined ? {} : {retryAfterMs: asked * 1000})});
        }
        setTailError(''); attempt = 0;
        await readEventStream(response, event => {
          if (event.id) lastId.current = event.id;
          if (event.event === 'reset') { setLines(prev => applyLogTailFrame(prev, {type: 'reset'})); return; }
          if (event.event !== 'log' && event.event !== 'message') return;
          let record: LogRecord;
          try { record = parseLogEvent(event.data); } catch { return; }
          const f = filter.current;
          if (f.category && record.category !== f.category) return;
          if (f.level && order.indexOf(record.level) > order.indexOf(f.level)) return;
          setLines(prev => applyLogTailFrame(prev, {type: 'record', record}));
        }, controller.signal);
      } catch (e) { /* fall through to the retry */ 
        if (!logTailShouldReconnect(controller.signal.aborted)) return;
        const status = (e as {status?: number})?.status;
        const asked = (e as {retryAfterMs?: number})?.retryAfterMs;
        if (status !== 429 || asked === undefined) attempt = Math.min(attempt + 1, 6);
        setTailError('Live updates paused. Reconnecting…');
        timer = setTimeout(() => void connect(), logTailRetryDelayMs(status, asked, attempt));
        return;
      }
      // The 30-minute stream ended normally: reconnect with backoff and resume
      // from the retained Last-Event-ID.
      if (!logTailShouldReconnect(controller.signal.aborted)) return;
      attempt = Math.min(attempt + 1, 6);
      setTailError('Live updates paused. Reconnecting…');
      timer = setTimeout(() => void connect(), logTailRetryDelayMs(undefined, undefined, attempt));
    };
    void connect();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [live, signedIn, api]);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => { if (live && box.current) box.current.scrollTop = box.current.scrollHeight; }, [lines, live]);
  return (
    <SettingsGroup title={t('web.logs.messages')} description={t('web.logs.messagesLede')}>
      {read.error ? <div style={{padding: 12}}><Notice tone={lines.length ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {action.error ? <div style={{padding: 12}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
      <div style={{padding: '12px 16px', display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center'}}>
        <Select hideLabel label={t('web.logs.level')} value={level} onChange={e => setLevel(e.target.value as LogLevel | '')} options={[{value: '', label: t('web.logs.allLevels')}, ...logLevels.map(l => ({value: l, label: t('web.logs.levelAndAbove', {level: `${l[0].toUpperCase()}${l.slice(1)}`})}))]} />
        <Select hideLabel label={t('feedback.category')} value={category} onChange={e => setCategory(e.target.value as LogCategory | '')} options={[{value: '', label: t('web.logs.everything')}, ...logCategories.map(c => ({value: c, label: c[0].toUpperCase() + c.slice(1)}))]} />
        <Switch checked={live} onCheckedChange={setLive} label={t('guide.live')} />
        {live ? <Status tone={tailError ? 'warning' : 'healthy'}>{tailError || t('web.logs.following')}</Status> : <Freshness at={read.at} onRefresh={read.reload} refreshing={read.loading} />}
      </div>
      <div ref={box} role="log" aria-live="off" style={{margin: '0 16px 12px', maxHeight: 360, overflow: 'auto', background: 'var(--surface-recess, var(--surface-diagnostics))', borderRadius: 10, padding: '8px 12px', fontFamily: 'var(--font-mono, ui-monospace, monospace)', fontSize: 12, lineHeight: 1.55}}>
        {lines.length ? lines.map(l => (
          <div key={l.sequence} style={{display: 'flex', gap: 12, whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>
            <span style={{color: 'var(--text-tertiary)', flexShrink: 0}}>{logTime(l.at)}</span>
            <span style={{color: levelTone[l.level], flexShrink: 0, width: 42}}>{l.level.toUpperCase()}</span>
            <span style={{color: 'var(--text-tertiary)', flexShrink: 0, width: 64}}>{l.category}</span>
            <span style={{color: l.level === 'error' ? 'var(--text-primary)' : 'var(--text-secondary)'}}>{l.message}</span>
          </div>
        )) : <Text variant="caption" tone="tertiary">{read.loading ? t('web.logs.loadingMessages') : t('web.logs.noMessages')}</Text>}
      </div>
    </SettingsGroup>
  );
}

/** A log line's time with its date: entries days apart must not read as the same afternoon. */
export function logTime(at: string): string {
  const d = new Date(at);
  return d.toLocaleString(undefined, {month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit'});
}

/**
 * Troubleshooting › "Record more detail for 30 minutes": the one control for extra detail
 * (it replaced a level menu, a debug window button and a component capture).
 */
export function DetailWindowRow() {
  const t = currentI18n().t;
  const deps = useServerDeps();
  const {form, state} = useServerForm<LogsDraft, LogSettings>('logs');
  const action = useAction();
  const until = state.status?.debugWindowUntil;
  const on = !!until && Date.parse(until) > Date.now();
  if (!state.status) return null;
  return (
    <SettingsRow label={t('settings.server.detailWindow')} help={on ? t('settings.server.detailWindow.until', {time: new Date(until!).toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'})}) : t('settings.server.detailWindow.help')} state={action.error || undefined}
      control={<Button size="sm" variant={on ? 'outline' : 'secondary'} label={on ? t('settings.server.detailWindow.stop') : t('settings.server.detailWindow.start')} loading={action.busy} onClick={() => void action.run(async () => { await setDetailWindow(deps, !on); await form.load(); })} />} />
  );
}

/**
 * Server › Logs & diagnostics › Server capabilities: every item the server sends, each
 * Available or Unavailable with the reason in plain words and the server's detail under
 * Technical details. Unknown ids render as a generic row. Hidden on servers without the
 * route (404). New rows stay hidden until the server sends them (behind presence).
 */
export function CapabilitiesGroup() {
  const {api} = useSession();
  const t = currentI18n().t;
  const [state, setState] = useState<{items?: readonly ServerCapability[]; missing?: boolean; error?: boolean}>({});
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let live = true;
    api.request<unknown>('/v1/admin/diagnostics/capabilities').then(
      raw => { if (live) setState({items: parseCapabilities(raw)}); },
      e => { if (live) setState((e as {status?: number})?.status === 404 ? {missing: true} : {error: true}); },
    );
    return () => { live = false; };
  }, [api, tick]);
  if (state.missing) return null;
  const find = (id: string) => state.items?.find(c => c.capability === id);
  const known = new Set<string>([...CAPABILITY_ROWS]);
  const extras = state.items?.filter(c => !known.has(c.capability)) ?? [];
  const row = (c: ServerCapability, label: string) => (
    <SettingsRow
      key={c.capability}
      label={label}
      help={c.available ? undefined : <>
        {t(capabilityReasonId(c.code) as never)}
        {c.code || c.detail ? <details style={{marginTop: 4}}><summary style={{cursor: 'pointer'}}>{t('web.capabilities.technical')}</summary><Text as="p" variant="caption" tone="tertiary" style={{fontFamily: 'var(--font-mono)', wordBreak: 'break-word'}}>{[c.code, c.detail].filter(Boolean).join(': ')}</Text></details> : null}
      </>}
      control={<Badge tone={c.available ? 'healthy' : 'warning'} dot>{c.available ? t('web.capabilities.available') : t('web.capabilities.unavailable')}</Badge>}
    />
  );
  return (
    <SettingsGroup title={t('web.capabilities.title')} description={t('web.capabilities.help')}>
      {state.error ? <div style={{padding: '0 16px 12px'}}><Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: () => setTick(n => n + 1)}}>{t('web.capabilities.loadFailed')}</Notice></div> : null}
      {state.items ? CAPABILITY_ROWS.map(id => {
        const c = find(id);
        const label = t(`web.capabilities.${id}` as never);
        if (!c) {
          if ((CAPABILITY_BEHIND_PRESENCE as readonly string[]).includes(id)) return null;
          return <SettingsRow key={id} label={label} help={t('web.capabilities.notChecked')} control={<Badge tone="neutral">{t('web.capabilities.unknown')}</Badge>} />;
        }
        return row(c, label);
      }) : null}
      {state.items ? extras.map(c => row(c, t('web.capabilities.otherLabel' as never, {id: c.capability} as never))) : null}
    </SettingsGroup>
  );
}
