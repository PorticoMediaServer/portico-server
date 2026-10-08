import {useMemo, useState} from 'react';
import {GuideWindowStore, type ChannelSource, type GuideChannelRow, type GuideDataSource, type GuideProgram} from '@core/guide/index.ts';
import {demoChannels, demoPrograms} from '@core/guide/testing/fixture.ts';
import {ChannelList, ChannelTimeline, GuideGrid, useGuideFormat, type Shared} from '../screens/channels/Channels';
import {Inset, Page, PageHeader, Segmented, Text} from '../ui';

/** Development only: the Channels guide and list against the demo fixture's shapes (no server). */
const library: GuideChannelRow[] = [
  {id: 'lib.movies', sourceId: 'library', kind: 'library', number: '', name: 'Movie Channel', group: '', favorite: true, tuneAvailable: true, recordAvailable: false, guide: 'full'},
  {id: 'lib.marathon', sourceId: 'library', kind: 'library', number: '', name: 'TV Marathons', group: '', favorite: false, tuneAvailable: false, tuneUnavailableReason: 'delivery-unavailable', recordAvailable: false, guide: 'full'},
];
const channels: GuideChannelRow[] = [...demoChannels.map(c => ({...c, sourceId: 'antenna'})), ...library];
const source = (slow: boolean): GuideDataSource => ({
  async channels(from, limit) { if (slow) await new Promise(r => setTimeout(r, 600)); return {items: channels.slice(from, from + limit), total: channels.length}; },
  async programs(ids, start, end) {
    if (slow) await new Promise(r => setTimeout(r, 900));
    const out: Record<string, GuideProgram[]> = {};
    for (const id of ids) out[id] = id.startsWith('lib.') ? demoPrograms('lantern.movies', start, end).map(p => ({...p, id: id + p.id, channelId: id})) : demoPrograms(id, start, end);
    return out;
  },
});
const sources: ChannelSource[] = [{id: 'antenna', name: 'Antenna', type: 'live', position: 0, recordAvailable: true}, {id: 'library', name: 'Library Channels', type: 'library', position: 1, recordAvailable: false}];

export function ChannelsLab() {
  const [view, setView] = useState<'guide' | 'list'>('guide');
  const [slow, setSlow] = useState<'fast' | 'slow'>('fast');
  const store = useMemo(() => new GuideWindowStore({source: source(slow === 'slow')}), [slow]);
  const format = useGuideFormat();
  const [last, setLast] = useState('');
  const [timeline, setTimeline] = useState<GuideChannelRow | null>(null);
  const lab = useMemo(() => source(slow === 'slow'), [slow]);
  const shared: Shared = {store, format, sources, entry: 'all', onOpen: sel => setLast(`open ${sel.channel.name}${sel.program ? ' · ' + sel.program.title : ''}`), onWatch: c => setLast(`watch ${c.name}`), onRecord: (c, p) => setLast(`record ${p.title} on ${c.name}`), channelMenu: () => [{id: 'fav', label: 'Add to Favorites', icon: 'star'}], onChannelMenu: (c, id) => setLast(`${id} ${c.name}`), onChannel: setTimeline, loadPrograms: (id, start, end, signal) => lab.programs([id], start, end, signal).then(r => r[id] ?? [])};
  const range = {start: Date.now() - 86_400_000, end: Date.now() + 8 * 86_400_000};
  return (
    <Page>
      <PageHeader title="Channels lab" actions={<><Segmented label="Speed" size="sm" options={[{id: 'fast' as const, label: 'Fast'}, {id: 'slow' as const, label: 'Slow'}]} value={slow} onChange={setSlow} /><Segmented label="View" size="sm" options={[{id: 'guide' as const, label: 'Guide'}, {id: 'list' as const, label: 'List'}]} value={view} onChange={setView} /></>} />
      <Inset><Text variant="caption" tone="tertiary">{last || '—'}</Text></Inset>
      {timeline ? <ChannelTimeline channel={timeline} format={format} range={range} loadPrograms={shared.loadPrograms} showSource sources={sources} onOpen={sel => setLast(`open ${sel.channel.name} · ${sel.program?.title ?? ''}`)} onWatch={c => setLast(`watch ${c.name}`)} onClose={() => setTimeline(null)} /> : null}
      {view === 'guide' ? <GuideGrid key={slow} {...shared} range={range} /> : <ChannelList key={slow} {...shared} />}
    </Page>
  );
}
