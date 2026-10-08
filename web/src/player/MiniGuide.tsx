import {useMemo} from 'react';
import type {GuideChannel} from '@core/channel-guide.ts';
import {useI18n} from '../app/i18n';
import {Dialog} from '../ui';
import {legacyChannel} from '@core/guide/index.ts';
import {surfChannel, surfNeighbours} from '../app/channels';
import {liveProgrammes, miniGuideRows} from './live';
import {nowNextProgrammes} from '@core/presentation/index.ts';
import s from './Player.module.css';

/**
 * MU4 FEAT-07: the mini guide over the video. Three rows — previous, current,
 * next channel, each with its now/next programme — tuning on select. The rows
 * come from the guide rows channel up/down steps through (watching from the
 * guide), else from the tune history. O(visible): it renders only these rows.
 */
export function MiniGuide({open, onOpenChange, onTune}: {open: boolean; onOpenChange: (open: boolean) => void; onTune: (channel: GuideChannel) => void}) {
  const i18n = useI18n();
  const rows = useMemo(() => {
    if (!open) return [];
    // Watching from the guide: the same rows channel up/down steps through (delta moves the surf).
    const around = surfNeighbours();
    if (around) {
      const now = Date.now();
      return ([[around.current, 0], [around.previous, -1], [around.next, 1]] as const)
        .map(([row, delta]) => ({channel: row ? legacyChannel(row) : undefined, delta}))
        .filter((x): x is {channel: GuideChannel; delta: 0 | -1 | 1} => !!x.channel)
        .map(({channel, delta}) => ({channel, delta, current: delta === 0, ...nowNextProgrammes(liveProgrammes(channel), now)}));
    }
    const {previous, current, next, programmes} = miniGuideRows();
    const ordered = [current, previous, next].filter((c): c is GuideChannel => !!c);
    const seen = new Set<string>();
    return ordered.filter(c => {
      const key = `${c.sourceId}/${c.id}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    }).map((channel, index) => ({channel, delta: 0 as const, current: index === 0, ...programmes(channel)}));
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={i18n.t('player.live.guide')} width={440}>
      <div role="menu" aria-label={i18n.t('player.live.guide')}>
        {rows.map(({channel, delta, current, now, next}) => {
          const head = [channel.number, channel.name].filter(Boolean).join(' · ');
          return (
            <button
              key={`${channel.sourceId}/${channel.id}`}
              type="button"
              role="menuitem"
              className={s.option}
              onClick={() => { const stepped = delta ? surfChannel(delta) : undefined; onTune((stepped ? legacyChannel(stepped) : undefined) ?? channel); onOpenChange(false); }}
            >
              <span>{head}{current ? ` · ${i18n.t('guide.onNow')}` : ''}<br />
                <span className={s.optionMeta}>{now ? `${now.title} · ${i18n.time(now.startMs)} – ${i18n.time(now.endMs)}` : i18n.t('guide.noInformation')}</span>
                {next ? <><br /><span className={s.optionMeta}>{i18n.t('guide.next', {title: next.title, time: i18n.time(next.startMs)})}</span></> : null}
              </span>
            </button>
          );
        })}
      </div>
    </Dialog>
  );
}
