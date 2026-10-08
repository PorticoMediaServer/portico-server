import React, {useEffect, useState} from 'react';
import {View} from 'react-native';
import {useAction, useClient, useQuery} from '../../data/DataProvider';
import type {HomeLayoutRow} from '../../data/types';
import {Sheet, ConfirmDialog} from '../../ui/Sheet';
import {Button, IconButton} from '../../ui/Button';
import {Loading, Notice, ListRow, Toggle} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Icon} from '../../ui/Icon';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';

/** Customize Home: reorder rows, hide rows, and add saved views as rows. */
export function CustomizeHome({visible, onClose}: {visible: boolean; onClose: () => void}) {
  const client = useClient();
  const m = useMetrics();
  const layout = useQuery(c => c.home.layout(), [visible]);
  const views = useQuery(c => c.saved.list('views'), [visible]);
  const [rows, setRows] = useState<HomeLayoutRow[]>([]);
  const [picking, setPicking] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);
  const {busy, run, error} = useAction();
  useEffect(() => { if (layout.data) setRows(layout.data); }, [layout.data]);
  const move = (i: number, d: number) => setRows(prev => { const j = i + d; if (j < 0 || j >= prev.length || prev[j]!.required || prev[i]!.required) return prev; const n = [...prev]; [n[i], n[j]] = [n[j]!, n[i]!]; return n; });
  const save = () => void run(async () => { await client.home.saveLayout(rows); onClose(); });
  const available = (views.data?.resources ?? []).filter(v => !rows.some(r => r.id === `view:${v.id}`));
  return (
    <>
      <Sheet visible={visible} onClose={onClose} title="Customize Home" subtitle="Choose which rows appear on Home and in what order." size="md"
        footer={<><Button label="Reset to default" variant="ghost" onPress={() => setConfirmReset(true)} /><View style={{flex: 1}} /><Button label="Cancel" variant="ghost" onPress={onClose} /><Button label="Save" variant="primary" loading={busy} onPress={save} /></>}>
        {error ? <Notice tone="error" body={error} /> : null}
        {layout.loading && !layout.data ? <Loading /> : null}
        {rows.map((r, i) => (
          <View key={r.id} style={{flexDirection: 'row', alignItems: 'center', gap: m.space.sm, opacity: r.hidden ? 0.6 : 1, borderBottomWidth: 1, borderColor: colors.lineSoft, paddingVertical: 2}}>
            <View style={{flexDirection: 'row'}}>
              <IconButton icon="chevronUp" label={`Move ${r.title} up`} size={m.controlSmall} disabled={i === 0 || r.required} onPress={() => move(i, -1)} />
              <IconButton icon="chevronDown" label={`Move ${r.title} down`} size={m.controlSmall} disabled={i === rows.length - 1 || r.required} onPress={() => move(i, 1)} />
            </View>
            <View style={{flex: 1}}>
              {r.required ? (
                <View style={{flexDirection: 'row', alignItems: 'center', gap: 8, paddingVertical: 12}}>
                  <Text variant="body" style={{flex: 1}}>{r.title}</Text>
                  <Icon name="lock" size={m.iconSmall} color={colors.textTertiary} />
                  <Text variant="caption" tone="tertiary">Always on Home</Text>
                </View>
              ) : <Toggle label={r.title} value={!r.hidden} onChange={v => setRows(prev => prev.map(x => (x.id === r.id ? {...x, hidden: !v} : x)))} />}
            </View>
            {r.savedView ? <IconButton icon="close" label={`Remove ${r.title} from Home`} size={m.controlSmall} onPress={() => setRows(prev => prev.filter(x => x.id !== r.id))} /> : null}
          </View>
        ))}
        {picking ? (
          <View style={{gap: 4}}>
            <Text variant="caption" tone="secondary">Choose a saved view to show on Home.</Text>
            {available.map(v => <ListRow key={v.id} icon="filter" title={v.title} subtitle={`${v.count} titles`} onPress={() => { setRows(prev => [...prev, {id: `view:${v.id}`, title: v.title, hidden: false, required: false, savedView: true}]); setPicking(false); }} />)}
            {!available.length ? <Text variant="caption" tone="tertiary">No saved views to add. Save one from a library’s filters.</Text> : null}
            <Button label="Cancel" size="sm" variant="ghost" onPress={() => setPicking(false)} />
          </View>
        ) : <Button label="Add a saved view" icon="plus" size="sm" variant="secondary" onPress={() => setPicking(true)} />}
      </Sheet>
      <ConfirmDialog visible={confirmReset} onClose={() => setConfirmReset(false)} title="Reset Home to the default layout?" body="Your row order and hidden rows are cleared." confirmLabel="Reset" onConfirm={() => void run(async () => { await client.home.resetLayout(); setConfirmReset(false); onClose(); })} />
    </>
  );
}
