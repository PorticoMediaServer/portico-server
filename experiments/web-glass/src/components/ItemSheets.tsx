import { useState } from 'react';
import { Check, Download, Lock, Search, Share2, Star, Unlock, Wand2 } from 'lucide-react';
import type { Download as DownloadT } from '../data/types';
import { useStartDownload } from '../services/queries';
import { useUsers } from '../services/queries';
import { Sheet, toast } from './overlay';
import { Avatar, Button, Segmented, Switch, cls } from './ui';
import { fmtBytes } from '../lib/hooks';

export function DownloadSheet({ open, onClose, itemId, title, sizeHint }: { open: boolean; onClose: () => void; itemId: string; title: string; sizeHint: number }) {
  const [q, setQ] = useState<DownloadT['quality']>('High (1080p)');
  const [smart, setSmart] = useState(true);
  const start = useStartDownload();
  const opts: { value: DownloadT['quality']; label: string; sub: string; mult: number }[] = [
    { value: 'Original', label: 'Original', sub: 'Exactly as stored on Harbor', mult: 2.6 },
    { value: 'High (1080p)', label: 'High', sub: '1080p · 10 Mbps · converted on the server', mult: 1 },
    { value: 'Medium (720p)', label: 'Medium', sub: '720p · 4 Mbps', mult: 0.45 },
    { value: 'Low (480p)', label: 'Low', sub: '480p · 2 Mbps · best for long flights', mult: 0.22 },
  ];
  return (
    <Sheet
      open={open}
      onClose={onClose}
      title={`Download ${title}`}
      size="sm"
      footer={
        <>
          <Button variant="plain" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="accent"
            icon={<Download />}
            loading={start.isPending}
            onClick={() =>
              start.mutate(
                { id: itemId, quality: q },
                {
                  onSuccess: () => {
                    toast(`Downloading ${title}`, <Download />);
                    onClose();
                  },
                },
              )
            }
          >
            Download
          </Button>
        </>
      }
    >
      <div className="choice-list" role="radiogroup" aria-label="Quality">
        {opts.map((o) => (
          <button key={o.value} role="radio" aria-checked={q === o.value} className={cls('choice', q === o.value && 'on')} onClick={() => setQ(o.value)}>
            <span className="choice-main">
              <span className="choice-title">{o.label}</span>
              <span className="choice-sub">{o.sub}</span>
            </span>
            <span className="choice-size tnum">{fmtBytes(sizeHint * o.mult)}</span>
            <span className="choice-dot">{q === o.value && <Check />}</span>
          </button>
        ))}
      </div>
      <div className="sheet-row">
        <div>
          <div className="row-title">Smart downloads</div>
          <div className="row-sub">For shows, keep the next 3 episodes downloaded and remove watched ones.</div>
        </div>
        <Switch checked={smart} onChange={setSmart} label="Smart downloads" />
      </div>
      <p className="t-caption" style={{ marginTop: 12 }}>
        Downloads stay available offline for 30 days and renew whenever you reconnect to Harbor. 41.2 GB free on this device.
      </p>
    </Sheet>
  );
}

export function RateSheet({ open, onClose, title, value, onRate }: { open: boolean; onClose: () => void; title: string; value?: number; onRate: (n: number) => void }) {
  const [hover, setHover] = useState<number | null>(null);
  const shown = hover ?? value ?? 0;
  return (
    <Sheet open={open} onClose={onClose} title={`Rate ${title}`} size="sm" compact>
      <div className="rate">
        <div className="stars" onMouseLeave={() => setHover(null)}>
          {[1, 2, 3, 4, 5].map((n) => (
            <button
              key={n}
              aria-label={`${n} star${n > 1 ? 's' : ''}`}
              className={cls('star', n <= shown && 'on')}
              onMouseEnter={() => setHover(n)}
              onClick={() => {
                onRate(n);
                toast(`Rated ${title} ${n} star${n > 1 ? 's' : ''}`, <Star />);
                onClose();
              }}
            >
              <Star fill={n <= shown ? 'currentColor' : 'none'} />
            </button>
          ))}
        </div>
        <p className="t-sub">Ratings are private to your profile and tune what Portico recommends.</p>
      </div>
    </Sheet>
  );
}

export function ShareSheet({ open, onClose, title }: { open: boolean; onClose: () => void; title: string }) {
  const users = useUsers();
  const [picked, setPicked] = useState<string[]>([]);
  const [note, setNote] = useState('');
  const people = (users.data ?? []).filter((u) => u.status === 'active' && u.id !== 'maya');
  return (
    <Sheet
      open={open}
      onClose={onClose}
      title={`Recommend ${title}`}
      size="sm"
      footer={
        <>
          <Button variant="plain" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="accent"
            icon={<Share2 />}
            disabled={!picked.length}
            onClick={() => {
              toast(`Recommended to ${picked.length} ${picked.length === 1 ? 'person' : 'people'}`, <Share2 />);
              onClose();
            }}
          >
            Send
          </Button>
        </>
      }
    >
      <p className="t-sub" style={{ marginBottom: 12 }}>
        They’ll see it in their notifications and on their Home screen. Only people who can access this library are listed.
      </p>
      <div className="share-people">
        {people.map((u) => (
          <button key={u.id} className={cls('share-person', picked.includes(u.id) && 'on')} onClick={() => setPicked((p) => (p.includes(u.id) ? p.filter((x) => x !== u.id) : [...p, u.id]))}>
            <Avatar name={u.name} color={u.avatarColor} size={52} />
            <span className="ellipsis">{u.name.split(' ')[0]}</span>
            {picked.includes(u.id) && (
              <span className="share-check">
                <Check />
              </span>
            )}
          </button>
        ))}
      </div>
      <div className="field" style={{ marginTop: 16 }}>
        <label htmlFor="note">Add a note</label>
        <input id="note" className="input" value={note} onChange={(e) => setNote(e.target.value)} placeholder="You have to see this one" />
      </div>
    </Sheet>
  );
}

export function EditMetadataSheet({ open, onClose, fields }: { open: boolean; onClose: () => void; fields: { title: string; sortTitle?: string; year?: number; summary: string; tagline?: string; rating?: string; genres?: string[]; studio?: string; art?: { poster?: string; backdrop?: string; logo?: string } } }) {
  const [tab, setTab] = useState<'general' | 'artwork' | 'advanced'>('general');
  const [f, setF] = useState(fields);
  const [locked, setLocked] = useState<Record<string, boolean>>({ summary: true });
  const L = ({ k }: { k: string }) => (
    <button type="button" className={cls('lock', locked[k] && 'on')} onClick={() => setLocked({ ...locked, [k]: !locked[k] })} aria-label={locked[k] ? `Unlock ${k}` : `Lock ${k}`} title={locked[k] ? 'Locked: refreshes won’t overwrite this' : 'Lock this field'}>
      {locked[k] ? <Lock /> : <Unlock />}
    </button>
  );
  return (
    <Sheet
      open={open}
      onClose={onClose}
      title={`Edit ${fields.title}`}
      size="lg"
      headerAction={<Segmented size="sm" value={tab} onChange={setTab} options={[{ value: 'general', label: 'General' }, { value: 'artwork', label: 'Artwork' }, { value: 'advanced', label: 'Advanced' }]} />}
      footer={
        <>
          <Button variant="plain" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="accent"
            onClick={() => {
              toast('Metadata saved. Locked fields won’t change on refresh.', <Check />);
              onClose();
            }}
          >
            Save
          </Button>
        </>
      }
    >
      {tab === 'general' && (
        <div className="form-grid">
          <div className="field span-2">
            <label>
              Title <L k="title" />
            </label>
            <input className="input" value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} />
          </div>
          <div className="field">
            <label>
              Sort title <L k="sort" />
            </label>
            <input className="input" value={f.sortTitle ?? ''} onChange={(e) => setF({ ...f, sortTitle: e.target.value })} />
          </div>
          <div className="field">
            <label>
              Year <L k="year" />
            </label>
            <input className="input tnum" value={f.year ?? ''} onChange={(e) => setF({ ...f, year: Number(e.target.value) || undefined })} />
          </div>
          {f.tagline !== undefined && (
            <div className="field span-2">
              <label>
                Tagline <L k="tagline" />
              </label>
              <input className="input" value={f.tagline} onChange={(e) => setF({ ...f, tagline: e.target.value })} />
            </div>
          )}
          <div className="field span-2">
            <label>
              Summary <L k="summary" />
            </label>
            <textarea className="input" rows={5} value={f.summary} onChange={(e) => setF({ ...f, summary: e.target.value })} />
          </div>
          <div className="field">
            <label>
              Content rating <L k="rating" />
            </label>
            <select className="input" value={f.rating} onChange={(e) => setF({ ...f, rating: e.target.value })}>
              {['G', 'PG', 'PG-13', 'R', 'NR', 'TV-Y', 'TV-Y7', 'TV-G', 'TV-PG', 'TV-14', 'TV-MA'].map((r) => (
                <option key={r}>{r}</option>
              ))}
            </select>
          </div>
          <div className="field">
            <label>
              Studio <L k="studio" />
            </label>
            <input className="input" value={f.studio ?? ''} onChange={(e) => setF({ ...f, studio: e.target.value })} />
          </div>
          <div className="field span-2">
            <label>
              Genres <L k="genres" />
            </label>
            <div className="tag-input">
              {(f.genres ?? []).map((g) => (
                <span key={g} className="tag">
                  {g}
                  <button type="button" aria-label={`Remove ${g}`} onClick={() => setF({ ...f, genres: f.genres?.filter((x) => x !== g) })}>
                    ×
                  </button>
                </span>
              ))}
              <input
                placeholder="Add genre"
                onKeyDown={(e) => {
                  const v = (e.target as HTMLInputElement).value.trim();
                  if (e.key === 'Enter' && v) {
                    e.preventDefault();
                    setF({ ...f, genres: [...(f.genres ?? []), v] });
                    (e.target as HTMLInputElement).value = '';
                  }
                }}
              />
            </div>
          </div>
        </div>
      )}
      {tab === 'artwork' && (
        <div className="art-choices">
          <p className="t-sub span-all">Artwork found by your metadata agents. Click to choose, or upload your own. Custom artwork is kept when metadata refreshes.</p>
          {[
            ['Poster', fields.art?.poster, 'poster'],
            ['Background', fields.art?.backdrop, 'wide'],
            ['Logo', fields.art?.logo, 'wide logo'],
          ]
            .filter(([, src]) => src)
            .map(([label, src, kind]) => (
              <button key={label} className={cls('art-choice', kind)} onClick={() => toast(`${label} kept as the primary artwork`)}>
                <span className="art-choice-img">
                  <img src={src} alt="" />
                </span>
                <span className="t-caption">{label} · primary</span>
              </button>
            ))}
          <button className="art-choice upload" onClick={() => toast('Drop an image here or choose one from your computer')}>
            <span className="art-choice-img">+</span>
            <span className="t-caption">Upload…</span>
          </button>
        </div>
      )}
      {tab === 'advanced' && (
        <div className="form-grid">
          <div className="field">
            <label>Metadata agent</label>
            <select className="input" defaultValue="tmdb">
              <option value="tmdb">The Movie Database</option>
              <option value="local">Local NFO only</option>
            </select>
          </div>
          <div className="field">
            <label>Original language audio</label>
            <select className="input" defaultValue="auto">
              <option value="auto">Use profile preference</option>
              <option value="orig">Always original</option>
            </select>
          </div>
          <div className="sheet-row span-2">
            <div>
              <div className="row-title">Hide from “Continue Watching”</div>
              <div className="row-sub">Useful for background films and screensaver loops.</div>
            </div>
            <Switch checked={false} onChange={() => undefined} label="Hide from continue watching" />
          </div>
          <div className="sheet-row span-2">
            <div>
              <div className="row-title">Allow in Kids profiles regardless of rating</div>
              <div className="row-sub">Overrides the content rating for this title only.</div>
            </div>
            <Switch checked={false} onChange={() => undefined} label="Allow in kids profiles" />
          </div>
        </div>
      )}
    </Sheet>
  );
}

export function FixMatchSheet({ open, onClose, title, year, candidates }: { open: boolean; onClose: () => void; title: string; year?: number; candidates: { title: string; year: number; source: string; id: string; image?: string; note?: string }[] }) {
  const [q, setQ] = useState(title);
  const [picked, setPicked] = useState(candidates[0]?.id);
  return (
    <Sheet
      open={open}
      onClose={onClose}
      title="Fix Match"
      footer={
        <>
          <Button variant="plain" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="accent"
            icon={<Wand2 />}
            onClick={() => {
              toast('Match updated. Refreshing artwork and details…', <Wand2 />);
              onClose();
            }}
          >
            Use This Match
          </Button>
        </>
      }
    >
      <div className="fix-search">
        <div className="search-field" style={{ flex: 1 }}>
          <Search />
          <input className="input" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Search title" />
        </div>
        <input className="input" style={{ width: 90 }} defaultValue={year} aria-label="Year" />
      </div>
      <div className="choice-list" style={{ marginTop: 12 }}>
        {candidates.map((c) => (
          <button key={c.id} className={cls('choice', picked === c.id && 'on')} onClick={() => setPicked(c.id)}>
            {c.image && <img src={c.image} alt="" className="choice-poster" />}
            <span className="choice-main">
              <span className="choice-title">
                {c.title} <span className="t-sub">({c.year})</span>
              </span>
              <span className="choice-sub">
                {c.source} · {c.id}
                {c.note ? ` · ${c.note}` : ''}
              </span>
            </span>
            <span className="choice-dot">{picked === c.id && <Check />}</span>
          </button>
        ))}
      </div>
    </Sheet>
  );
}
