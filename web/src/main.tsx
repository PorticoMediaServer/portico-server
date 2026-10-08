import './app/polyfills';
import './bridge/invitation-location';
import {createRoot} from 'react-dom/client';
import './ui/tokens.css';
import './ui/base.css';
import {App} from './app/App';

// No StrictMode: the headless core services (activation polling, content
// selection, playback) own real side effects and are not written for
// double-invoked effects. Lifecycle correctness is tested against them
// directly instead.
createRoot(document.getElementById('root')!).render(<App />);
