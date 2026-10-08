import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@fontsource-variable/inter';
import './styles/tokens.css';
import './styles/base.css';
import './styles/glass.css';
import './styles/controls.css';
import './styles/overlay.css';
import './styles/media.css';
import './styles/shell.css';
import './styles/chrome.css';
import './styles/pages.css';
import './styles/player.css';
import './styles/console.css';
import './styles/auth.css';
import './styles/detail.css';
import './styles/music.css';
import './styles/photos.css';
import './styles/live.css';
import { App } from './app/App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
