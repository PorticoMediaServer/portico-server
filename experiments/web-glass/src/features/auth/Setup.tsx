import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import {
  ArrowLeft,
  BookHeadphones,
  Check,
  ChevronRight,
  Film,
  Folder,
  FolderOpen,
  Globe,
  HardDrive,
  Images,
  Music2,
  Radio,
  Router,
  Sparkles,
  Trash2,
  Tv,
  Video,
} from 'lucide-react';
import { Button, Switch, cls } from '../../components/ui';
import { Sheet } from '../../components/overlay';
import { useSession } from '../../state/session';

const STEPS = ['welcome', 'account', 'libraries', 'remote', 'livetv', 'done'] as const;
type Step = (typeof STEPS)[number];
const LABELS: Record<Step, string> = { welcome: 'Server', account: 'Owner', libraries: 'Libraries', remote: 'Remote Access', livetv: 'Live TV', done: 'Finish' };

const KINDS = [
  { id: 'movies', label: 'Movies', icon: Film },
  { id: 'shows', label: 'TV Shows', icon: Tv },
  { id: 'anime', label: 'Anime', icon: Sparkles },
  { id: 'music', label: 'Music', icon: Music2 },
  { id: 'audiobooks', label: 'Audiobooks', icon: BookHeadphones },
  { id: 'photos', label: 'Photos', icon: Images },
  { id: 'homevideos', label: 'Home Videos', icon: Video },
];

const TREE: Record<string, string[]> = {
  '/': ['volume1', 'volume2', 'mnt', 'home'],
  '/volume1': ['media', 'downloads', 'docker'],
  '/volume1/media': ['Movies', 'UHD', 'TV', 'Anime', 'Music', 'Audiobooks', 'DVR'],
  '/volume2': ['photos', 'home-videos', 'backups'],
  '/mnt': ['backup'],
  '/home': ['maya'],
};

interface Lib {
  kind: string;
  name: string;
  path: string;
}

export function Setup() {
  const { step = 'welcome' } = useParams();
  const navigate = useNavigate();
  const signIn = useSession((s) => s.signIn);
  const chooseServer = useSession((s) => s.chooseServer);
  const cur = (STEPS.includes(step as Step) ? step : 'welcome') as Step;
  const idx = STEPS.indexOf(cur);
  const [name, setName] = useState('Harbor');
  const [owner, setOwner] = useState({ name: 'Maya Lindqvist', email: 'maya@lindqvist.family', password: '' });
  const [libs, setLibs] = useState<Lib[]>([
    { kind: 'movies', name: 'Movies', path: '/volume1/media/Movies' },
    { kind: 'shows', name: 'TV Shows', path: '/volume1/media/TV' },
  ]);
  const [browse, setBrowse] = useState<string | null>(null);
  const [remote, setRemote] = useState(true);
  const [upnp, setUpnp] = useState(false);
  const [domain, setDomain] = useState('harbor.lindqvist.family');
  const [tuner, setTuner] = useState(true);
  const [scan, setScan] = useState(0);
  const next = () => navigate(`/setup/${STEPS[idx + 1]}`, { viewTransition: true });
  const back = () => navigate(idx === 0 ? '/welcome' : `/setup/${STEPS[idx - 1]}`, { viewTransition: true });

  useEffect(() => {
    if (cur !== 'done') return;
    setScan(0);
    const t = setInterval(() => setScan((s) => Math.min(1, s + 0.035 + Math.random() * 0.03)), 180);
    return () => clearInterval(t);
  }, [cur]);

  const canNext = cur === 'welcome' ? name.trim().length > 1 : cur === 'account' ? owner.email.includes('@') && owner.password.length >= 8 : cur === 'libraries' ? libs.length > 0 : true;

  return (
    <div className="auth setup">
      <div className="auth-center">
        <div className="setup-card glass glass-thick">
          <ol className="setup-steps" aria-label="Setup progress">
            {STEPS.map((s, i) => (
              <li key={s} className={cls(i < idx && 'done', i === idx && 'on')}>
                <span className="setup-dot">{i < idx ? <Check /> : i + 1}</span>
                <span className="setup-step-label">{LABELS[s]}</span>
              </li>
            ))}
          </ol>

          <div className="setup-body" key={cur}>
            {cur === 'welcome' && (
              <>
                <div className="setup-hero">
                  <img src="/brand/portico-appicon-dark.svg" alt="" className="setup-icon" />
                  <h1 className="auth-title">Let’s set up your server</h1>
                  <p className="auth-sub">Portico Server 3.2.1 is running on this machine: Intel Core i5-13500 · 64 GB · Intel Quick Sync. Setup takes about two minutes.</p>
                </div>
                <div className="field">
                  <label htmlFor="sname">Server name</label>
                  <input id="sname" className="input" value={name} onChange={(e) => setName(e.target.value)} />
                  <span className="field-hint">Shown on TVs, phones and in invitations. You can change it later.</span>
                </div>
              </>
            )}
            {cur === 'account' && (
              <>
                <h1 className="auth-title">Create the owner account</h1>
                <p className="auth-sub">The owner manages libraries, people and settings. It stays on this server — no cloud account required.</p>
                <div className="setup-grid">
                  <div className="field">
                    <label htmlFor="oname">Name</label>
                    <input id="oname" className="input" value={owner.name} onChange={(e) => setOwner({ ...owner, name: e.target.value })} />
                  </div>
                  <div className="field">
                    <label htmlFor="oemail">Email</label>
                    <input id="oemail" className="input" type="email" value={owner.email} onChange={(e) => setOwner({ ...owner, email: e.target.value })} />
                  </div>
                  <div className="field span-2">
                    <label htmlFor="opw">Password</label>
                    <input id="opw" className="input" type="password" autoComplete="new-password" value={owner.password} onChange={(e) => setOwner({ ...owner, password: e.target.value })} placeholder="At least 8 characters" />
                    <PasswordMeter value={owner.password} />
                  </div>
                </div>
                <p className="field-hint">After setup you can add a passkey and two-step verification in Sign-In & Security.</p>
              </>
            )}
            {cur === 'libraries' && (
              <>
                <h1 className="auth-title">Add your libraries</h1>
                <p className="auth-sub">Point Portico at folders on this server. We’ll organize everything and fetch artwork, cast and descriptions.</p>
                <div className="setup-libs">
                  {libs.map((l, i) => {
                    const K = KINDS.find((k) => k.id === l.kind)!;
                    return (
                      <div key={i} className="setup-lib">
                        <span className="setup-lib-icon">
                          <K.icon />
                        </span>
                        <div className="setup-lib-main">
                          <div className="setup-lib-name">{l.name}</div>
                          <div className="t-mono">{l.path}</div>
                        </div>
                        <button className="icon-btn sm plain" aria-label={`Remove ${l.name}`} onClick={() => setLibs(libs.filter((_, j) => j !== i))}>
                          <Trash2 />
                        </button>
                      </div>
                    );
                  })}
                </div>
                <div className="setup-kinds">
                  {KINDS.map((k) => (
                    <button key={k.id} className="setup-kind" onClick={() => setBrowse(k.id)}>
                      <k.icon />
                      <span>{k.label}</span>
                    </button>
                  ))}
                </div>
              </>
            )}
            {cur === 'remote' && (
              <>
                <h1 className="auth-title">Watch away from home</h1>
                <p className="auth-sub">Remote access lets you and the people you share with stream from anywhere. You can turn it off any time.</p>
                <div className="group">
                  <div className="row">
                    <span className="row-icon" style={{ background: '#2f86bb' }}>
                      <Globe />
                    </span>
                    <div className="row-main">
                      <div className="row-title">Remote access</div>
                      <div className="row-sub">Stream outside your home network</div>
                    </div>
                    <Switch checked={remote} onChange={setRemote} label="Remote access" />
                  </div>
                  <div className="row">
                    <span className="row-icon" style={{ background: '#3fae6b' }}>
                      <Router />
                    </span>
                    <div className="row-main">
                      <div className="row-title">Configure my router automatically (UPnP)</div>
                      <div className="row-sub">Off: forward TCP 32500 to 192.168.1.20 yourself</div>
                    </div>
                    <Switch checked={upnp} onChange={setUpnp} label="UPnP" disabled={!remote} />
                  </div>
                </div>
                {remote && (
                  <>
                    <div className="field" style={{ marginTop: 18 }}>
                      <label htmlFor="domain">Custom domain (optional)</label>
                      <input id="domain" className="input mono" value={domain} onChange={(e) => setDomain(e.target.value)} />
                      <span className="field-hint">We’ll request a free Let’s Encrypt certificate and renew it automatically.</span>
                    </div>
                    <div className="check-list">
                      <div className="check ok">
                        <Check /> Port 32500 reachable from the internet
                      </div>
                      <div className="check ok">
                        <Check /> Upload speed 300 Mbps — enough for ~24 remote HD streams
                      </div>
                    </div>
                  </>
                )}
              </>
            )}
            {cur === 'livetv' && (
              <>
                <h1 className="auth-title">Live TV & DVR</h1>
                <p className="auth-sub">We found a tuner on your network. Add it to watch and record over-the-air channels.</p>
                <button className={cls('tuner-card', tuner && 'on')} onClick={() => setTuner(!tuner)}>
                  <span className="tuner-icon">
                    <Radio />
                  </span>
                  <span className="setup-lib-main">
                    <span className="setup-lib-name">HDHomeRun FLEX 4K</span>
                    <span className="t-sub">192.168.1.44 · 4 tuners · ATSC 3.0 · 7 channels found</span>
                  </span>
                  <span className={cls('tuner-check', tuner && 'on')}>{tuner && <Check />}</span>
                </button>
                <div className="field" style={{ marginTop: 18 }}>
                  <label htmlFor="zip">Guide location</label>
                  <input id="zip" className="input" defaultValue="04101 · Portland, ME (Antenna)" />
                  <span className="field-hint">14 days of listings, refreshed every 12 hours. You can add IPTV playlists and XMLTV sources later.</span>
                </div>
              </>
            )}
            {cur === 'done' && (
              <div className="setup-done">
                <div className={cls('done-ring', scan >= 1 && 'complete')}>
                  <svg viewBox="0 0 120 120" aria-hidden="true">
                    <circle cx="60" cy="60" r="52" />
                    <circle cx="60" cy="60" r="52" className="fg" style={{ strokeDashoffset: 327 * (1 - scan) }} />
                  </svg>
                  {scan >= 1 ? <Check className="done-check" /> : <span className="tnum">{Math.round(scan * 100)}%</span>}
                </div>
                <h1 className="auth-title">{scan >= 1 ? `${name} is ready` : 'Scanning your libraries…'}</h1>
                <p className="auth-sub">
                  {scan >= 1
                    ? 'Artwork and descriptions will keep filling in for a few minutes. Invite your household from the Server Console.'
                    : `Found ${Math.round(scan * 1284)} items so far. You can start watching while we finish.`}
                </p>
              </div>
            )}
          </div>

          <div className="setup-foot">
            <Button variant="ghost" icon={<ArrowLeft />} onClick={back}>
              Back
            </Button>
            {cur === 'done' ? (
              <Button
                variant="accent"
                size="lg"
                onClick={() => {
                  signIn();
                  chooseServer('srv-harbor');
                  navigate('/profiles');
                }}
              >
                Go to {name}
              </Button>
            ) : (
              <Button variant="accent" size="lg" onClick={next} disabled={!canNext}>
                {cur === 'livetv' ? 'Finish Setup' : 'Continue'}
                <ChevronRight />
              </Button>
            )}
          </div>
        </div>
      </div>
      {browse && (
        <FolderPicker
          kind={browse}
          onClose={() => setBrowse(null)}
          onPick={(path) => {
            const K = KINDS.find((k) => k.id === browse)!;
            setLibs([...libs, { kind: browse, name: K.label, path }]);
            setBrowse(null);
          }}
        />
      )}
    </div>
  );
}

function PasswordMeter({ value }: { value: string }) {
  const score = Math.min(4, (value.length >= 8 ? 1 : 0) + (/[A-Z]/.test(value) ? 1 : 0) + (/\d/.test(value) ? 1 : 0) + (/[^A-Za-z0-9]/.test(value) || value.length > 14 ? 1 : 0));
  const label = ['Too short', 'Weak', 'Fair', 'Good', 'Strong'][score];
  return (
    <div className="pw-meter" aria-live="polite">
      <div className="pw-bars">
        {[0, 1, 2, 3].map((i) => (
          <i key={i} className={cls(i < score && `s${score}`)} />
        ))}
      </div>
      <span>{value ? label : 'Use a passphrase you don’t use anywhere else'}</span>
    </div>
  );
}

function FolderPicker({ kind, onClose, onPick }: { kind: string; onClose: () => void; onPick: (p: string) => void }) {
  const defaults: Record<string, string> = { movies: '/volume1/media', shows: '/volume1/media', anime: '/volume1/media', music: '/volume1/media', audiobooks: '/volume1/media', photos: '/volume2', homevideos: '/volume2' };
  const [path, setPath] = useState(defaults[kind] ?? '/');
  const children = TREE[path] ?? [];
  const label = KINDS.find((k) => k.id === kind)?.label;
  const parts = path.split('/').filter(Boolean);
  return (
    <Sheet
      open
      onClose={onClose}
      title={`Choose a folder for ${label}`}
      footer={
        <>
          <span className="t-mono" style={{ marginRight: 'auto', alignSelf: 'center' }}>
            {path}
          </span>
          <Button variant="accent" onClick={() => onPick(path)} disabled={path === '/'}>
            Add Folder
          </Button>
        </>
      }
    >
      <div className="crumbs">
        <button onClick={() => setPath('/')}>
          <HardDrive className="inline-ic" /> Harbor
        </button>
        {parts.map((p, i) => (
          <button key={i} onClick={() => setPath('/' + parts.slice(0, i + 1).join('/'))}>
            / {p}
          </button>
        ))}
      </div>
      <div className="group" style={{ marginTop: 12 }}>
        {children.length === 0 && <div className="row t-sub">No subfolders. Add this folder or go back.</div>}
        {children.map((c) => (
          <button key={c} className="row has-icon" onClick={() => setPath(`${path === '/' ? '' : path}/${c}`)}>
            <span className="row-icon" style={{ background: '#1c3a57' }}>
              {TREE[`${path === '/' ? '' : path}/${c}`] ? <FolderOpen /> : <Folder />}
            </span>
            <span className="row-main">
              <span className="row-title">{c}</span>
            </span>
            <ChevronRight className="row-chev" />
          </button>
        ))}
      </div>
    </Sheet>
  );
}
