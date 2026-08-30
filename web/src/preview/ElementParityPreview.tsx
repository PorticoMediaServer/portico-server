import { useEffect, useState } from 'react';
import { PorticoAction, PorticoField, ProviderAction } from './ElementParityControls';
import './element-parity-preview.css';

type PreviewView = 'auth' | 'states';

function previewView(): PreviewView {
  return window.location.hash === '#states' ? 'states' : 'auth';
}

function PreviewNavigation({ view }: { view: PreviewView }) {
  return <nav className="parity-preview-navigation" aria-label="Preview sections">
    <a aria-current={view === 'auth' ? 'page' : undefined} href="#auth">Authentication</a>
    <a aria-current={view === 'states' ? 'page' : undefined} href="#states">Component states</a>
  </nav>;
}

function AuthenticationPreview() {
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  return <main className="auth-surface parity-preview-surface">
    <PreviewNavigation view="auth" />
    <section className="auth-panel runtime-panel parity-auth-panel" aria-labelledby="parity-auth-title">
      <img className="auth-wordmark" src="/brand/portico-wordmark-white.svg" alt="Portico" />
      <h1 id="parity-auth-title">Sign in to Portico</h1>
      <p className="runtime-intro">Sign in using your Portico Account credentials.</p>
      <div className="parity-provider-stack" aria-label="Sign-in providers">
        <ProviderAction provider="google">Sign in with Google</ProviderAction>
        <ProviderAction provider="apple">Sign in with Apple</ProviderAction>
      </div>
      <div className="parity-identity-divider"><span>or sign in with email</span></div>
      <form onSubmit={(event) => event.preventDefault()}>
        <PorticoField label="Username or email" placeholder="Username or email" autoComplete="username" value={login} onChange={(event) => setLogin(event.target.value)} />
        <PorticoField label="Password" placeholder="Password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} />
        <PorticoAction type="submit">Sign in</PorticoAction>
        <div className="parity-utility-links"><button type="button">Forgot password?</button><span aria-hidden="true">·</span><button type="button">Create an account</button></div>
      </form>
      <p className="parity-legal">By continuing, you agree to Portico’s <a href="https://getportico.tv/terms/" target="_blank" rel="noreferrer">Terms of Use</a> and acknowledge the <a href="https://getportico.tv/privacy/" target="_blank" rel="noreferrer">Privacy Policy</a>.</p>
    </section>
  </main>;
}

function StateGallery() {
  return <main className="auth-surface parity-preview-surface parity-gallery-surface">
    <PreviewNavigation view="states" />
    <section className="auth-panel runtime-panel parity-state-gallery" aria-labelledby="parity-gallery-title">
      <img className="auth-wordmark" src="/brand/portico-wordmark-white.svg" alt="Portico" />
      <h1 id="parity-gallery-title">Component states</h1>
      <p className="runtime-intro">The proposed literal control contract, shown without changing Web page composition.</p>
      <div className="parity-state-grid">
        <article><h2>Credential fields</h2><PorticoField label="Default" placeholder="Username or email" /><PorticoField label="Hover" state="hover" defaultValue="justin" /><PorticoField label="Focus" state="focus" defaultValue="justin@getportico.tv" /><PorticoField label="Invalid" state="invalid" defaultValue="not-an-email" /><PorticoField label="Disabled" disabled defaultValue="Unavailable" /></article>
        <article><h2>Actions</h2><PorticoAction>Default action</PorticoAction><PorticoAction className="is-hover">Hover action</PorticoAction><PorticoAction className="is-focus">Focus action</PorticoAction><PorticoAction disabled>Disabled action</PorticoAction><ProviderAction provider="google">Sign in with Google</ProviderAction><ProviderAction provider="apple">Sign in with Apple</ProviderAction></article>
      </div>
    </section>
  </main>;
}

export function ElementParityPreview() {
  const [view, setView] = useState(previewView);
  useEffect(() => {
    const update = () => setView(previewView());
    window.addEventListener('hashchange', update);
    return () => window.removeEventListener('hashchange', update);
  }, []);
  return view === 'states' ? <StateGallery /> : <AuthenticationPreview />;
}
