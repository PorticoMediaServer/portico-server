import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { ArrowRight, ChevronRight, Fingerprint, Globe, HardDrive, Lock, Plus, QrCode, Server, ShieldCheck, Wifi, WifiOff } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { movies, shows, albums } from '../../data/fixtures/index';
import { api } from '../../services/client';
import { useServers } from '../../services/queries';
import { useSession } from '../../state/session';
import { Button, Skel, cls } from '../../components/ui';

/** A slow, tilted wall of the household's own posters behind the sign-in flow. */
function PosterWall() {
  const cols = useMemo(() => {
    const art = [...movies.map((m) => m.art.poster!), ...shows.map((s) => s.art.poster!), ...albums.map((a) => a.art.square!)].filter(Boolean);
    return Array.from({ length: 9 }, (_, c) => Array.from({ length: 8 }, (_, r) => art[(c * 7 + r * 5 + c * r) % art.length]));
  }, []);
  return (
    <div className="wall" aria-hidden="true">
      <div className="wall-grid">
        {cols.map((col, c) => (
          <div key={c} className={cls('wall-col', c % 2 ? 'down' : '')} style={{ animationDuration: `${80 + c * 7}s` }}>
            {[...col, ...col].map((src, r) => (
              <img key={r} src={src} alt="" loading={r < 5 ? 'eager' : 'lazy'} className={src.includes('/cover/') ? 'sq' : ''} />
            ))}
          </div>
        ))}
      </div>
      <div className="wall-shade" />
    </div>
  );
}

export function Welcome() {
  const navigate = useNavigate();
  const signedIn = useSession((s) => s.signedIn);
  useEffect(() => {
    if (signedIn) navigate('/profiles', { replace: true });
  }, [signedIn, navigate]);
  return (
    <div className="auth welcome">
      <PosterWall />
      <div className="auth-center">
        <div className="welcome-card">
          <img className="welcome-mark" src="/brand/portico-wordmark-mono-white.svg" alt="Portico" />
          <h1 className="welcome-title">Everything you love, served from home.</h1>
          <p className="welcome-sub">Films, shows, anime, music, audiobooks, photos and live TV — from your own server, beautifully, on every screen.</p>
          <div className="welcome-actions">
            <Button variant="primary" size="xl" to="/signin">
              Sign In
            </Button>
            <Button variant="glass" size="xl" to="/setup/welcome">
              Set Up a Server
            </Button>
          </div>
          <div className="welcome-found">
            <span className="found-dot" />
            Found <strong>Harbor</strong> on this network · 192.168.1.20
          </div>
        </div>
      </div>
      <footer className="auth-foot">
        <span>No account needed on your network</span>
        <span>·</span>
        <span>Your library never leaves your server</span>
        <span>·</span>
        <Link to="/screens">Screen index</Link>
      </footer>
    </div>
  );
}

export function SignIn() {
  const navigate = useNavigate();
  const signIn = useSession((s) => s.signIn);
  const [step, setStep] = useState<'password' | 'mfa'>('password');
  const [email, setEmail] = useState('maya@lindqvist.family');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [code, setCode] = useState(['', '', '', '', '', '']);
  const inputs = useRef<(HTMLInputElement | null)[]>([]);

  const finish = () => {
    signIn();
    navigate('/servers', { replace: true, viewTransition: true });
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const r = await api.signIn(email, password);
      if (!r.ok) setError(r.error ?? 'Sign-in failed');
      else if (r.needsMfa) {
        setStep('mfa');
        setTimeout(() => inputs.current[0]?.focus(), 50);
      } else finish();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Couldn’t reach the server.');
    } finally {
      setBusy(false);
    }
  };

  const verify = async (digits: string[]) => {
    setBusy(true);
    setError(null);
    const r = await api.verifyMfa(digits.join('')).catch(() => ({ ok: false, error: 'Couldn’t reach the server.' }));
    setBusy(false);
    if (r.ok) finish();
    else {
      setError(r.error ?? 'Invalid code');
      setCode(['', '', '', '', '', '']);
      inputs.current[0]?.focus();
    }
  };

  const passkey = async () => {
    setBusy(true);
    const r = await api.signInWithPasskey();
    setBusy(false);
    if (r.ok) finish();
  };

  return (
    <div className="auth">
      <PosterWall />
      <div className="auth-center">
        <div className="auth-card glass glass-thick">
          <img className="auth-mark" src="/brand/portico-symbol-mono-white.svg" alt="" />
          {step === 'password' ? (
            <>
              <h1 className="auth-title">Sign in to Portico</h1>
              <p className="auth-sub">Use your Portico account, or the account your server owner created for you.</p>
              <button className="btn btn-primary btn-lg btn-block" onClick={passkey} disabled={busy}>
                <Fingerprint />
                Continue with Passkey
              </button>
              <div className="auth-or">
                <span>or use your password</span>
              </div>
              <form className="auth-form" onSubmit={submit} noValidate>
                <div className="field">
                  <label htmlFor="email">Email</label>
                  <input id="email" className="input" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} />
                </div>
                <div className="field">
                  <label htmlFor="password">Password</label>
                  <input id="password" className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="At least 6 characters" />
                </div>
                {error && (
                  <div className="field-error" role="alert">
                    {error}
                  </div>
                )}
                <Button variant="accent" size="lg" block type="submit" loading={busy}>
                  Sign In
                </Button>
              </form>
              <div className="auth-links">
                <button className="linkish" onClick={() => setError('We sent a reset link to ' + email + '. It expires in 30 minutes.')}>
                  Forgot password?
                </button>
                <span className="auth-qr">
                  <QrCode /> Sign in on a TV? Visit <strong>harbor.lindqvist.family/link</strong>
                </span>
              </div>
            </>
          ) : (
            <>
              <h1 className="auth-title">Two-step verification</h1>
              <p className="auth-sub">Enter the 6-digit code from your authenticator app for {email}.</p>
              <div className="otp" onPaste={(e) => {
                const digits = e.clipboardData.getData('text').replace(/\D/g, '').slice(0, 6).split('');
                if (digits.length === 6) {
                  setCode(digits);
                  verify(digits);
                }
              }}>
                {code.map((d, i) => (
                  <input
                    key={i}
                    ref={(el) => {
                      inputs.current[i] = el;
                    }}
                    className="otp-box"
                    inputMode="numeric"
                    maxLength={1}
                    value={d}
                    aria-label={`Digit ${i + 1}`}
                    onChange={(e) => {
                      const v = e.target.value.replace(/\D/g, '').slice(-1);
                      const next = [...code];
                      next[i] = v;
                      setCode(next);
                      if (v && i < 5) inputs.current[i + 1]?.focus();
                      if (next.every((x) => x)) verify(next);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Backspace' && !code[i] && i > 0) inputs.current[i - 1]?.focus();
                    }}
                  />
                ))}
              </div>
              {busy && (
                <div className="auth-busy">
                  <span className="spinner" /> Verifying…
                </div>
              )}
              {error && (
                <div className="field-error" role="alert" style={{ textAlign: 'center' }}>
                  {error}
                </div>
              )}
              <div className="auth-links center">
                <button className="linkish" onClick={() => setStep('password')}>
                  Use a different account
                </button>
                <button className="linkish" onClick={() => setError('Recovery codes are in your account’s Security settings on another device.')}>
                  Use a recovery code
                </button>
              </div>
            </>
          )}
        </div>
      </div>
      <footer className="auth-foot">
        <ShieldCheck /> <span>Passwords are verified by your server. Portico never sees them.</span>
      </footer>
    </div>
  );
}

export function Servers() {
  const servers = useServers();
  const choose = useSession((s) => s.chooseServer);
  const signedIn = useSession((s) => s.signedIn);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [manual, setManual] = useState(false);
  const [addr, setAddr] = useState('');
  useEffect(() => {
    if (!signedIn) navigate('/welcome', { replace: true });
  }, [signedIn, navigate]);
  return (
    <div className="auth">
      <div className="auth-center">
        <div className="auth-card wide glass glass-thick">
          <div className="auth-icon">
            <Server />
          </div>
          <h1 className="auth-title">Choose a server</h1>
          <p className="auth-sub">Signed in as maya@lindqvist.family. You can switch servers any time from Settings.</p>
          <div className="server-list">
            {servers.isLoading &&
              Array.from({ length: 3 }, (_, i) => (
                <div key={i} className="server-row">
                  <Skel w={44} h={44} r={12} />
                  <div style={{ flex: 1, display: 'grid', gap: 6 }}>
                    <Skel h={14} w="40%" />
                    <Skel h={11} w="70%" />
                  </div>
                </div>
              ))}
            {servers.data?.map((s) => (
              <button
                key={s.id}
                className={cls('server-row', !s.online && 'offline')}
                disabled={!s.online}
                onClick={() => {
                  choose(s.id);
                  qc.invalidateQueries();
                  navigate('/profiles', { viewTransition: true });
                }}
              >
                <span className="server-icon">{s.local ? <HardDrive /> : <Globe />}</span>
                <span className="server-main">
                  <span className="server-name">
                    {s.name}
                    {s.id === 'srv-harbor' && <span className="badge green">This network</span>}
                  </span>
                  <span className="server-sub">
                    {s.owner} · {s.address}
                  </span>
                </span>
                <span className="server-status">
                  {s.online ? (
                    <>
                      <Wifi /> {s.latencyMs} ms
                    </>
                  ) : (
                    <>
                      <WifiOff /> Offline
                    </>
                  )}
                </span>
                {s.online && <ChevronRight className="row-chev" />}
              </button>
            ))}
          </div>
          {manual ? (
            <form
              className="manual-server"
              onSubmit={(e) => {
                e.preventDefault();
                setManual(false);
              }}
            >
              <input className="input" autoFocus placeholder="https://server.example.com:32500" value={addr} onChange={(e) => setAddr(e.target.value)} aria-label="Server address" />
              <Button variant="plain" type="submit" disabled={!addr}>
                Connect
              </Button>
            </form>
          ) : (
            <div className="auth-links center">
              <button className="linkish" onClick={() => setManual(true)}>
                <Plus className="inline-ic" /> Add a server by address
              </button>
              <Link className="linkish" to="/setup/welcome">
                Set up a new server <ArrowRight className="inline-ic" />
              </Link>
            </div>
          )}
        </div>
      </div>
      <footer className="auth-foot">
        <Lock /> <span>Connections to remote servers are end-to-end TLS.</span>
      </footer>
    </div>
  );
}
