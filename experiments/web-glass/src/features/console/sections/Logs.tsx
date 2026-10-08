import { useEffect, useMemo, useRef, useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useVirtualizer } from '@tanstack/react-virtual';
import { Download, Pause, Play, Search, X } from 'lucide-react';
import type { LogLevel, LogLine } from '../../../services/api';
import { api } from '../../../services/client';
import { logLine, LOG_COUNT } from '../../../data/fixtures/server';
import { Button, ErrorState, cls } from '../../../components/ui';
import { toast } from '../../../components/overlay';

const LEVELS: LogLevel[] = ['ERROR', 'WARN', 'INFO', 'DEBUG'];
const SOURCES = ['all', 'http', 'stream', 'transcoder', 'scanner', 'metadata', 'dvr', 'dlna', 'auth', 'network', 'storage', 'backup', 'db'];
const PAGE = 400;

export function Logs() {
  const [levels, setLevels] = useState<LogLevel[]>(['ERROR', 'WARN', 'INFO']);
  const [source, setSource] = useState('all');
  const [text, setText] = useState('');
  const [debounced, setDebounced] = useState('');
  const [tail, setTail] = useState(true);
  const [live, setLive] = useState<LogLine[]>([]);
  const [open, setOpen] = useState<number | null>(null);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(text), 200);
    return () => clearTimeout(t);
  }, [text]);

  const q = useInfiniteQuery({
    queryKey: ['admin', 'logs', levels, source, debounced],
    queryFn: ({ pageParam }) => api.getLogs({ levels, source: source === 'all' ? undefined : source, text: debounced || undefined, offset: pageParam, limit: PAGE }),
    initialPageParam: 0,
    getNextPageParam: (last) => (last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
  });

  // live tail: synthesize new lines in the same format
  useEffect(() => {
    setLive([]);
    if (!tail) return;
    let n = LOG_COUNT;
    const t = setInterval(() => {
      const line = logLine(n++ + 7, Date.now());
      if (!levels.includes(line.level) || (source !== 'all' && line.source !== source) || (debounced && !line.msg.toLowerCase().includes(debounced.toLowerCase()))) return;
      setLive((l) => [line, ...l].slice(0, 200));
    }, 1100);
    return () => clearInterval(t);
  }, [tail, levels, source, debounced]);

  const rows = useMemo(() => [...live, ...(q.data?.pages.flatMap((p) => p.items) ?? [])], [live, q.data]);
  const total = (q.data?.pages[0]?.total ?? 0) + live.length;
  const v = useVirtualizer({ count: rows.length, getScrollElement: () => box.current, estimateSize: () => 26, overscan: 20 });
  const items = v.getVirtualItems();
  useEffect(() => {
    const last = items[items.length - 1];
    if (last && last.index > rows.length - 60 && q.hasNextPage && !q.isFetchingNextPage) q.fetchNextPage();
  }, [items, rows.length, q]);

  const toggle = (l: LogLevel) => setLevels((x) => (x.includes(l) ? x.filter((y) => y !== l) : [...x, l]));

  return (
    <div className="cstack">
      <div className="log-tools">
        <div className="cfilter search-field">
          <Search />
          <input className="input sm" value={text} onChange={(e) => setText(e.target.value)} placeholder="Filter messages  ( / )" aria-label="Filter log messages" />
          {text && (
            <button className="icon-btn sm plain clear" onClick={() => setText('')} aria-label="Clear filter">
              <X />
            </button>
          )}
        </div>
        <div className="lvl-toggles" role="group" aria-label="Levels">
          {LEVELS.map((l) => (
            <button key={l} className={cls('lvl', l.toLowerCase(), levels.includes(l) && 'on')} onClick={() => toggle(l)} aria-pressed={levels.includes(l)}>
              {l}
            </button>
          ))}
        </div>
        <select className="input sm" value={source} onChange={(e) => setSource(e.target.value)} aria-label="Source" style={{ width: 150 }}>
          {SOURCES.map((s) => (
            <option key={s} value={s}>
              {s === 'all' ? 'All sources' : s}
            </option>
          ))}
        </select>
        <span style={{ flex: 1 }} />
        <span className="t-caption tnum">{q.isLoading ? 'Loading…' : `${total.toLocaleString('en-US')} lines`}</span>
        <Button size="sm" variant={tail ? 'tinted' : 'plain'} icon={tail ? <Pause /> : <Play />} onClick={() => setTail(!tail)}>
          {tail ? 'Live' : 'Paused'}
        </Button>
        <Button size="sm" variant="plain" icon={<Download />} onClick={() => toast('portico-server-logs.zip (14 days, 38 MB) is downloading')}>
          Export
        </Button>
      </div>
      {q.isError ? (
        <ErrorState error={q.error} retry={() => q.refetch()} />
      ) : (
        <div className="logbox" ref={box} role="log" aria-live="off">
          <div style={{ height: v.getTotalSize(), position: 'relative' }}>
            {items.map((it) => {
              const l = rows[it.index];
              return (
                <div
                  key={it.key}
                  ref={v.measureElement}
                  data-index={it.index}
                  className={cls('logline', l.level.toLowerCase(), open === l.id && 'open', it.index < live.length && 'fresh')}
                  style={{ transform: `translateY(${it.start}px)` }}
                  onClick={() => setOpen(open === l.id ? null : l.id)}
                >
                  <span className="lt tnum">{new Date(l.t).toLocaleTimeString('en-US', { hour12: false })}.{String(l.t % 1000).padStart(3, '0')}</span>
                  <span className="ll">{l.level}</span>
                  <span className="ls">{l.source}</span>
                  <span className="lm">{l.msg}</span>
                </div>
              );
            })}
          </div>
          {!q.isLoading && rows.length === 0 && <div className="ctable-empty">No log lines match these filters.</div>}
        </div>
      )}
    </div>
  );
}
