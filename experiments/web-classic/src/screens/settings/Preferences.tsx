import {useState} from 'react';
import {useClient, useTopic} from '@/data/DataProvider';
import type {PrefDef} from '@/data/types';
import {usePrefs} from '@/app/prefs';
import {Button, IconButton} from '@/ui/Button';
import {Chip} from '@/ui/Bits';
import {Segmented, Select, Switch} from '@/ui/Form';
import {ConfirmDialog, Menu, useToast} from '@/ui/Overlay';
import {Group} from '@/ui/Page';
import s from './Settings.module.css';

const LANGS: Record<string, string> = {en: 'English', fr: 'French', de: 'German', es: 'Spanish', it: 'Italian', ja: 'Japanese', pt: 'Portuguese', nl: 'Dutch', sv: 'Swedish', ko: 'Korean'};
const ORDER = ['Playback', 'Subtitles', 'Languages', 'Quality', 'Streaming', 'Music', 'Privacy', 'Search', 'Notifications', 'Language and region'];

/**
 * Preferences as the server publishes them, grouped. Profile preferences
 * follow this profile to every device; device preferences apply to this browser.
 */
export function PreferencesSection() {
  const client = useClient();
  useTopic(['prefs']);
  const toast = useToast();
  const {prefs, update} = usePrefs();
  const [reset, setReset] = useState(false);
  const defs = client.prefs.defs().filter(d => d.group !== 'Appearance');
  const groups = ORDER.filter(g => defs.some(d => d.group === g));
  const set = async (d: PrefDef, v: boolean | number | string | string[]) => { await client.prefs.set(d.key, v); toast.show(`${d.label}: saved`, {icon: 'check', duration: 1800}); };
  return (
    <>
      <Group title="Appearance" lede="These apply to this browser.">
        <div className={s.prefRow}><span><span className="t-strong">Card size</span><span className="t-caption c-3" style={{display: 'block'}}>Posters and artwork in rows and grids</span></span><span className={s.prefControl}><Segmented label="Card size" value={prefs.cardSize} onChange={v => update({cardSize: v})} options={[{value: 'small', label: 'Small'}, {value: 'medium', label: 'Medium'}, {value: 'large', label: 'Large'}]} /></span></div>
        <div className={s.prefRow}><span><span className="t-strong">Show backdrops</span><span className="t-caption c-3" style={{display: 'block'}}>Artwork behind title pages</span></span><span className={s.prefControl}><Switch checked={prefs.showBackdrops} onChange={v => update({showBackdrops: v})} label="Show backdrops" /></span></div>
        <div className={s.prefRow}><span><span className="t-strong">Reduce motion</span><span className="t-caption c-3" style={{display: 'block'}}>Shorter transitions and no slow zoom on pictures. Portico also follows your system setting.</span></span><span className={s.prefControl}><Switch checked={prefs.reduceMotion} onChange={v => update({reduceMotion: v})} label="Reduce motion" /></span></div>
        <div className={s.prefRow}><span><span className="t-strong">Featured billboard on Home</span></span><span className={s.prefControl}><Switch checked={prefs.homeHero} onChange={v => update({homeHero: v})} label="Featured billboard" /></span></div>
      </Group>
      {groups.map(g => (
        <Group key={g} title={g} lede={g === 'Playback' ? 'Preferences follow this profile to every device, except those marked This browser.' : undefined}>
          {defs.filter(d => d.group === g).map(d => (
            <div key={d.key} className={s.prefRow}>
              <span><span className="t-strong">{d.label}</span>{d.scope === 'device' ? <span className={s.scope}>This browser</span> : null}{d.help ? <span className="t-caption c-3" style={{display: 'block', marginTop: 2}}>{d.help}</span> : null}</span>
              <span className={s.prefControl}><PrefControl def={d} value={client.prefs.get(d.key)} onChange={v => void set(d, v)} /></span>
            </div>
          ))}
        </Group>
      ))}
      <Group title="Recommendations">
        <div className={s.prefRow}><span><span className="t-strong">Reset recommendations</span><span className="t-caption c-3" style={{display: 'block'}}>Forget your likes, dislikes and Not interested choices. Your ratings and watch history stay.</span></span><span className={s.prefControl}><Button variant="secondary" label="Reset…" onClick={() => setReset(true)} /></span></div>
      </Group>
      <ConfirmDialog open={reset} onOpenChange={setReset} title="Reset recommendations?" body="Picks for you starts over from what you watch and rate. Titles you marked Not interested can be suggested again." action="Reset" tone="danger" onConfirm={async () => { await client.prefs.resetRecommendations(); setReset(false); toast.show('Recommendations reset', {icon: 'check'}); }} />
    </>
  );
}

function PrefControl({def, value, onChange}: {def: PrefDef; value: unknown; onChange: (v: boolean | number | string | string[]) => void}) {
  if (def.type === 'toggle') return <Switch checked={value === true} onChange={onChange} label={def.label} />;
  if (def.type === 'languages') {
    const list = (value as string[]) ?? [];
    return (
      <span className={s.langs}>
        {list.map((l, i) => <Chip key={l} onRemove={() => onChange(list.filter(x => x !== l))} title={`Remove ${LANGS[l] ?? l}`}>{i + 1}. {LANGS[l] ?? l}</Chip>)}
        <Menu label="Add a language" items={Object.entries(LANGS).filter(([k]) => !list.includes(k)).map(([k, v]) => ({id: k, label: v, onSelect: () => onChange([...list, k])}))} trigger={<IconButton size="sm" variant="secondary" icon="plus" label="Add a language" />} />
      </span>
    );
  }
  const options = (def.options ?? []).map(o => ({value: String(o.value), label: o.label}));
  return <Select compact ariaLabel={def.label} value={String(value)} onChange={v => { const o = def.options?.find(x => String(x.value) === v); onChange(typeof o?.value === 'number' ? o.value : v); }} options={options} />;
}
