import {useEffect, useState} from 'react';
import type {EvidenceRecord, SupportExport} from '@core/console.ts';
import {sentence, useAction, useConsole, useRead, when} from '../../admin/console';
import {Badge, Button, Checkbox, Freshness, Notice, SettingsGroup, SettingsRow, StateView, Surface, Tabs, Text} from '../../ui';
import {useI18n, type MessageId} from '../../app/i18n';
import s from './Server.module.css';

type Lane = 'runtime' | 'client' | 'audit';
const lanes: readonly {id: Lane; label: MessageId}[] = [{id: 'runtime', label: 'web.logs.laneServer'}, {id: 'client', label: 'web.logs.laneClient'}, {id: 'audit', label: 'web.logs.laneAudit'}];
const severityTone = (v?: string): 'danger' | 'warning' | 'neutral' | 'accent' => (v === 'error' || v === 'critical' ? 'danger' : v === 'warn' || v === 'warning' ? 'warning' : v === 'info' ? 'accent' : 'neutral');

/** Troubleshooting › Server records: what the server noted, by lane. */
export function RecordsPanel() {
  const client = useConsole();
  const {t} = useI18n();
  const [lane, setLane] = useState<Lane>('runtime');
  const [cursor, setCursor] = useState('');
  useEffect(() => setCursor(''), [lane]);
  const read = useRead(() => client.records(lane, cursor), [client, lane, cursor]);
  const items = read.data?.items ?? [];
  return (
    <Surface padless>
      <div className={s.panelHead} style={{padding: '12px 16px 0'}}>
        <span className={s.panelTitle}>{t('web.logs.records')}</span>
        <Freshness at={read.at} onRefresh={read.reload} refreshing={read.loading} />
      </div>
      <Tabs items={lanes.map(l => ({id: l.id, label: t(l.label)}))} value={lane} onChange={id => setLane(id as Lane)} label={t('web.logs.records')} />
      {read.error && !read.data ? <div style={{padding: 16}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {read.data && !items.length ? <StateView inline icon="console" title={t('settings.server.records.none')} /> : null}
      <div>
        {items.map(r => <Record key={r.sequence} record={r} />)}
      </div>
      {read.data?.nextCursor || cursor ? <div style={{display: 'flex', gap: 8, padding: 12}}><Button size="sm" variant="ghost" label={t('web.activity.latest')} disabled={!cursor} onClick={() => setCursor('')} /><Button size="sm" variant="ghost" label={t('web.hostedAccount.older')} disabled={!read.data?.nextCursor} onClick={() => setCursor(read.data!.nextCursor)} /></div> : null}
    </Surface>
  );
}

function Record({record}: {record: EvidenceRecord}) {
  const {t} = useI18n();
  return (
    <div className={s.stream}>
      <div className={s.streamCopy}>
        <div style={{display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap'}}>
          <Badge tone={severityTone(record.severity)}>{sentence(record.severity ?? 'record')}</Badge>
          <Text variant="bodyStrong">{record.code ? sentence(record.code) : record.action ? sentence(record.action) : t('web.logs.recordFallback')}</Text>
          {record.component ? <Text variant="caption" tone="tertiary">{sentence(record.component)}</Text> : null}
        </div>
        <div className={s.streamMeta}><span>{when(record.at)}</span>{record.target ? <span>{record.target}</span> : null}{record.fields ? Object.entries(record.fields).map(([k, v]) => <span key={k}>{sentence(k)} {v}</span>) : null}</div>
      </div>
      <Text variant="mono" tone="muted" style={{fontSize: 11}}>#{record.sequence}</Text>
    </div>
  );
}

/** Troubleshooting › Support export: a file for whoever is helping, with what it leaves out said plainly. */
export function SupportExportPanel() {
  const client = useConsole();
  const {t} = useI18n();
  const action = useAction();
  const [hours, setHours] = useState(24);
  const [chosen, setChosen] = useState<string[]>(['runtime', 'audit']);
  const [result, setResult] = useState<SupportExport>();
  const toggle = (c: string) => setChosen(v => (v.includes(c) ? v.filter(x => x !== c) : [...v, c]));
  const download = () => {
    if (!result) return;
    const blob = new Blob([JSON.stringify(result, null, 2)], {type: 'application/json'});
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `portico-support-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.json`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  return (
    <SettingsGroup title={t('web.logs.exportTitle')} description={t('web.logs.exportLede')}>
      <SettingsRow label={t('web.logs.timeWindow')} control={<div style={{display: 'flex', gap: 8}}>{[1, 6, 24, 72].map(h => <Button key={h} size="sm" variant="outline" selected={hours === h} label={h < 24 ? `${h} h` : `${h / 24} d`} onClick={() => setHours(h)} />)}</div>} start />
      <SettingsRow label={t('web.logs.include')} control={<div style={{display: 'flex', gap: 16, flexWrap: 'wrap'}}>{['runtime', 'client', 'audit', 'snapshot'].map(c => <Checkbox key={c} checked={chosen.includes(c)} onCheckedChange={() => toggle(c)} label={c === 'runtime' ? t('web.logs.includeServer') : c === 'client' ? t('web.logs.includeClient') : c === 'audit' ? t('web.logs.includeAudit') : t('web.logs.includeSnapshot')} />)}</div>} start />
      <SettingsRow label={t('web.logs.prepareExport')} control={<Button variant="primary" size="sm" label={t('web.logs.prepare')} loading={action.busy} disabled={!chosen.length} onClick={() => void action.run(async () => setResult(await client.createExport(Date.now() - hours * 3600000, Date.now(), chosen)))} />} />
      {result ? (
        <SettingsRow label={t('web.logs.readyToDownload')} help={t('web.logs.readyHelp', {included: result.manifest.included.map(sentence).join(', ') || t('web.logs.exportNothing'), excluded: result.manifest.excluded.length ? result.manifest.excluded.map(sentence).join(', ') : 'none', redaction: sentence(result.manifest.redaction), expires: when(result.expiresAt)})} control={<Button variant="secondary" size="sm" icon="download" label={t('web.logs.downloadJson')} onClick={download} />} />
      ) : null}
      {action.error ? <div style={{padding: '0 16px 12px'}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
    </SettingsGroup>
  );
}
