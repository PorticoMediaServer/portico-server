import {createRoot} from 'react-dom/client';
import {DownloadsService} from '@core/downloads-service.ts';
import {ServerDownloads} from '../../src/screens/saved/Downloads';
import {SavedOperations} from '../../src/screens/saved/operations';
import {IconSprite} from '../../src/kit/Icon';
import '../../src/kit/tokens.css';
import '../../src/kit/fonts.css';
import '../../src/bootstrap.css';

// Component-only failure proof: the actual DownloadsService reads through a
// real loopback proxy whose upstream refuses the connection. No rows, native
// store, authority, successful API response or application shell is fabricated.
let current = true;
const operations = new SavedOperations(() => current);
const service = new DownloadsService({async request<T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {method, body: body === undefined ? undefined : JSON.stringify(body), signal});
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return await response.json() as T;
}});
const root = createRoot(document.getElementById('root')!);
const errors: unknown[] = [];
const unexpected = () => { throw new Error('No preparation was offered in this failure state.'); };
const proof = {
  service, errors,
  mount() {
    root.render(<><IconSprite /><ServerDownloads service={service} operations={operations} online artwork={() => undefined}
      effects={{openItem: unexpected, saveFile: async () => unexpected(), reportError: error => errors.push(error)}} /></>);
  },
  dispose() { current = false; operations.dispose(); service.dispose(); root.unmount(); },
};
Object.assign(window, {savedDownloadsComponentProof: proof});
