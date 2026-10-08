import React, {useState} from 'react';
import {View} from 'react-native';
import {useAction, useClient, useTopic} from '../../data/DataProvider';
import type {PrefDef} from '../../data/types';
import {Button, IconButton} from '../../ui/Button';
import {Group, ListRow, Toggle, Badge} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {ChoiceSheet, ConfirmDialog} from '../../ui/Sheet';
import {useMetrics} from '../../theme/layout';
import {useToast} from '../../app/toast';
import {languageNames} from '../../data/fixtures/prefs';

/** Preferences from the server's registry: playback, subtitles, languages, streaming, quality, music, privacy, appearance. */
export function PreferencesSection() {
  const client = useClient();
  useTopic(['prefs']);
  const m = useMetrics();
  const toast = useToast();
  const defs = client.prefs.defs();
  const groups = [...new Set(defs.map(d => d.group))];
  const [pick, setPick] = useState<PrefDef>();
  const [langPick, setLangPick] = useState<string>();
  const [reset, setReset] = useState(false);
  const {busy, run} = useAction();
  const value = (d: PrefDef) => client.prefs.get(d.key);
  return (
    <>
      <View style={{flexDirection: 'row', gap: 8, flexWrap: 'wrap'}}><Badge label="Saved to your profile" tone="accent" icon="user" /><Badge label={m.isTV ? 'This TV' : 'This device'} icon={m.isTV ? 'tvDevice' : 'phone'} /></View>
      {groups.map(g => (
        <Group key={g} title={g}>
          {defs.filter(d => d.group === g).map(d => {
            const scope = d.scope === 'device' ? (m.isTV ? 'This TV' : 'This device') : undefined;
            if (d.type === 'toggle') return <View key={d.key} style={{paddingHorizontal: m.isTV ? 0 : 14}}><Toggle label={d.label} help={[d.help, scope].filter(Boolean).join(' · ')} value={value(d) === true} onChange={v => void client.prefs.set(d.key, v)} /></View>;
            if (d.type === 'languages') {
              const list = (value(d) as string[]) ?? [];
              return (
                <View key={d.key} style={{paddingHorizontal: 14, paddingVertical: 10, gap: 8}}>
                  <Text variant="body">{d.label}</Text>
                  <Text variant="caption" tone="tertiary">{d.help}</Text>
                  {list.length ? list.map((l, i) => (
                    <View key={l} style={{flexDirection: 'row', alignItems: 'center', gap: 8}}>
                      <Text variant="bodyStrong" style={{flex: 1}}>{i + 1}. {languageNames[l] ?? l}</Text>
                      <IconButton icon="chevronUp" label="Move up" size={m.controlSmall} disabled={i === 0} onPress={() => { const n = [...list]; [n[i - 1], n[i]] = [n[i]!, n[i - 1]!]; void client.prefs.set(d.key, n); }} />
                      <IconButton icon="chevronDown" label="Move down" size={m.controlSmall} disabled={i === list.length - 1} onPress={() => { const n = [...list]; [n[i + 1], n[i]] = [n[i]!, n[i + 1]!]; void client.prefs.set(d.key, n); }} />
                      <IconButton icon="close" label={`Remove ${languageNames[l]}`} size={m.controlSmall} onPress={() => void client.prefs.set(d.key, list.filter(x => x !== l))} />
                    </View>
                  )) : <Text variant="caption" tone="tertiary">No languages chosen</Text>}
                  <View><Button size="sm" icon="plus" label="Add language" variant="secondary" onPress={() => setLangPick(d.key)} /></View>
                </View>
              );
            }
            const opt = d.options?.find(o => String(o.value) === String(value(d)));
            return <ListRow key={d.key} title={d.label} subtitle={[d.help, scope].filter(Boolean).join(' · ') || undefined} value={opt?.label ?? String(value(d))} onPress={() => setPick(d)} />;
          })}
        </Group>
      ))}
      <Group title="Recommendations">
        <ListRow icon="refresh" title="Reset recommendations" subtitle="Start over with what Portico recommends to you." onPress={() => setReset(true)} />
      </Group>
      {pick ? <ChoiceSheet visible onClose={() => setPick(undefined)} title={pick.label} subtitle={pick.help} options={(pick.options ?? []).map(o => ({value: String(o.value), label: o.label}))} value={String(value(pick))} onChange={v => { const o = pick.options?.find(x => String(x.value) === v); void client.prefs.set(pick.key, o ? o.value : v); }} /> : null}
      {langPick ? <ChoiceSheet visible onClose={() => setLangPick(undefined)} title="Add language" options={Object.entries(languageNames).filter(([k]) => !((client.prefs.get(langPick) as string[]) ?? []).includes(k)).map(([k, v]) => ({value: k, label: v}))} onChange={v => void client.prefs.set(langPick, [...((client.prefs.get(langPick) as string[]) ?? []), v])} /> : null}
      <ConfirmDialog visible={reset} onClose={() => setReset(false)} title="Reset recommendations?" body="Portico will forget what it has learned about your taste. Your watch history, ratings and lists stay." confirmLabel="Reset" busy={busy} onConfirm={() => void run(async () => { await client.prefs.resetRecommendations(); setReset(false); toast({message: 'Recommendations reset.'}); })} />
    </>
  );
}
