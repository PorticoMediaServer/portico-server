import {useState} from 'react';
import {createRoot} from 'react-dom/client';
import {ToastHost, type ToastData} from '../../src/kit/overlays';
import '../../src/kit/tokens.css';
import '../../src/kit/fonts.css';
import '../../src/bootstrap.css';

// Controlled lifetime proof only; this fixture establishes no product content or server effect.
const events: {id: string; elapsed: number}[] = [];
function Fixture() {
  const [toast, setToast] = useState<ToastData | null>(null);
  Object.assign(window, {toastLifetime: {events, show(id: string, durationMs?: number, action = false) {
    const started = performance.now();
    Object.assign(window, {toastLifetimeStarted: started});
    setToast({id, message: 'Lifetime fixture', durationMs, ...(action ? {action: {label: 'Fixture action', onAction() {}}} : {})});
  }}});
  return <ToastHost toast={toast} bottomInset={24} onDismiss={id => {
    events.push({id, elapsed: performance.now() - (window as unknown as {toastLifetimeStarted: number}).toastLifetimeStarted});
    setToast(null);
  }} />;
}
createRoot(document.getElementById('root')!).render(<Fixture />);
