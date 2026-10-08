import {useNavigate} from '@tanstack/react-router';
import {useSession} from '../app/session';
import {useI18n} from '../app/i18n';
import {BrandMark, Button, ListRow, cx, useCompact} from '../ui';
import {Wordmark} from '../ui/Brand';
import {HostedChooser} from '../screens/auth/HostedChooser';
import {DirectProfiles} from '../screens/auth/DirectProfiles';
import {DirectStep, SignInEnded} from '../screens/auth/SignIn';
import {DeviceBlock, type DeviceAccess} from '../app/device';
import s from './Shell.module.css';
import l from './ServerlessShell.module.css';

/**
 * Justin's rule for a Portico Account sign-in: it lands in the normal shell at once, never on a
 * separate "choose a server" page. Until a server is open, the shell's frame shows what it can
 * (the account, Settings for the account, sign-out) and its content area shows the way into a
 * server: the account's one server opens by itself, several open the last one used, otherwise
 * the list (with Refresh Servers); a server's profile chooser, an approval wait or a specific
 * failure appears here too, centred, with its own action.
 *
 * INT gate 3: the same frame holds a server that is waiting for its owner to approve this
 * browser, or refused it (`device`), and says so when a Portico Account's server sign-in ended.
 */
export function ServerlessShell({device}: {device?: DeviceAccess}) {
  const {t} = useI18n();
  const session = useSession();
  const navigate = useNavigate();
  const compact = useCompact();
  const account = session.hosted?.account;
  const name = account?.displayName || account?.username || session.local?.username || t('web.shell.profileFallback');
  const content = device ? <DeviceBlock access={device} /> : session.direct.step ? <DirectStep inShell /> : session.direct.pending ? <DirectProfiles onDone={() => undefined} /> : <HostedChooser inShell />;
  const signOut = device ? session.signOut : () => void session.signOutAccount();
  return (
    <div className={cx(s.frame, compact && l.compact)}>
      {!compact ? (
        <nav className={s.rail} aria-label={t('web.shell.primary')}>
          <span className={s.brand}><BrandMark size={26} /><span><Wordmark height={16} /></span></span>
          <div className={s.railGroup}>
            <ListRow icon="home" title={t('web.shell.home')} />
          </div>
          <div className={s.railSpacer} />
          <div className={s.railFoot}>
            {device ? <ListRow icon="account" title={name} /> : <ListRow icon="account" title={name} subtitle={t('web.account.openAccount')} onClick={() => void navigate({to: '/account', search: {}})} />}
            <ListRow icon="signOut" title={t('action.signOut')} onClick={signOut} />
          </div>
        </nav>
      ) : null}
      <div className={s.main}>
        <main className={cx(s.content, l.content)}>
          {!device && session.signInHint ? <div className={l.panel}><SignInEnded /></div> : null}
          <div className={l.panel}>{content}</div>
          {!device && !session.direct.pending && !session.direct.step ? (
            <div className={l.links}>
              <Button variant="link" label={t('web.serverless.direct')} onClick={() => void session.signOutAccount()} />
            </div>
          ) : null}
        </main>
      </div>
    </div>
  );
}
