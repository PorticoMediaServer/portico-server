import {createRoot} from 'react-dom/client';
import {IconSprite} from '../../kit/Icon';
import {MobileServerChooser, type MobileChooserAttempt, type MobileChooserRow} from '../../screens/accounts/Servers.route';
import '../../kit/tokens.css';
import '../../kit/fonts.css';
import '../../bootstrap.css';

// Drawn white-box facts live only in this DEV specimen. Runtime rows never invent
// presence or ownership that the saved-session store does not hold.
if (!import.meta.env.DEV) throw new Error('Mobile chooser specimens require development.');
const rows: readonly MobileChooserRow[] = [
  {id: 'portico-demo', label: 'Portico demo', signIn: 'portico', selected: false, status: {kind: 'online'}, owner: true},
  {id: 'the-cottage', label: 'The cottage', signIn: 'portico', selected: false, status: {kind: 'online'}},
  {id: 'dads-server', label: 'Dad’s server', signIn: 'portico', selected: false, status: {kind: 'seen', at: '2026-10-12T12:00:00Z'}},
  {id: 'lake-house', label: 'Lake house', signIn: 'portico', selected: false, status: {kind: 'never'}},
];
const record = (kind: string, value?: string) => {document.documentElement.dataset.chooserAction = JSON.stringify({kind, value});};
// The frame was drawn with day-month dates; the DEV specimen shows that same authored state.
const frameSeen = (iso: string) => new Intl.DateTimeFormat('en-GB', {day: 'numeric', month: 'short', timeZone: 'UTC'}).format(new Date(iso));
function specimen() {
  const params = new URLSearchParams(location.search), frame = params.get('frame'), choice = params.get('state') ?? frame;
  if (choice === '6b' || choice === 'waiting') return {attempt: {kind: 'waiting', row: rows[1]!, seconds: 20, total: 20} as MobileChooserAttempt};
  if (choice === '6c' || choice === 'unreachable') return {attempt: {kind: 'unreachable', row: rows[2]!} as MobileChooserAttempt};
  if (choice === 'opening') return {attempt: {kind: 'opening', row: rows[1]!} as MobileChooserAttempt};
  if (choice === 'loading') return {attempt: {kind: 'loading'} as MobileChooserAttempt};
  if (choice === 'loadFailed') return {attempt: {kind: 'loadFailed'} as MobileChooserAttempt};
  return {attempt: undefined};
}
const {attempt} = specimen();
createRoot(document.getElementById('root')!).render(<><IconSprite /><MobileServerChooser rows={rows} initial="N" attempt={attempt} formatSeen={frameSeen}
  onChoose={row => record('choose', row.id)} onRefresh={() => record('refresh')} onLocal={() => record('local')}
  onDifferentAccount={() => record('different-account')} onCheckNow={() => record('check-now')} onOther={() => record('other-server')} /></>);
