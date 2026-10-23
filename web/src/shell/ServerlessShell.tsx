import {useNavigate} from '@tanstack/react-router';
import {useSession} from '../app/session';
import {useI18n} from '../app/i18n';
import {BrandMark, Button, ListRow, cx, useCompact} from '../ui';
import {Wordmark} from '../ui/Brand';
import {HostedChooser} from '../screens/auth/HostedChooser';
import {DirectProfiles} from '../screens/auth/DirectProfiles';
import {DirectStep, SignInEnded} from '../screens/auth/SignIn';
import s from './Shell.module.css';
import l from './ServerlessShell.module.css';

/** A Portico Account without an open server keeps the server chooser in the shell. */
export function ServerlessShell() {
  const {t} = useI18n();
  const session = useSession();
  const navigate = useNavigate();
  const compact = useCompact();
  const account = session.hosted?.account;
  const name = account?.displayName || account?.username || session.local?.username || t('web.shell.profileFallback');
  const content = session.direct.step ? <DirectStep inShell /> : session.direct.pending ? <DirectProfiles onDone={() => undefined} /> : <HostedChooser inShell />;
  const signOut = () => void session.signOutAccount();
  return (
    <div className={cx(s.frame, compact && l.compact)}>
      {!compact ? (
        <nav className={s.rail} aria-label={t('web.shell.primary')}>
          <span className={s.brand}><BrandMark size={28} className={s.brandMark} /><span className={s.brandWord}><Wordmark height={28} /></span></span>
          <div className={s.railGroup}>
            <ListRow icon="home" title={t('web.shell.home')} />
          </div>
          <div className={s.railSpacer} />
          <div className={s.railFoot}>
            <ListRow icon="account" title={name} subtitle={t('web.account.openAccount')} onClick={() => void navigate({to: '/account', search: {}})} />
            <ListRow icon="signOut" title={t('action.signOut')} onClick={signOut} />
          </div>
        </nav>
      ) : null}
      <div className={s.main}>
        <main className={cx(s.content, l.content)}>
          {session.signInHint ? <div className={l.panel}><SignInEnded /></div> : null}
          <div className={l.panel}>{content}</div>
          {!session.direct.pending && !session.direct.step ? (
            <div className={l.links}>
              <Button variant="link" label={t('web.serverless.direct')} onClick={() => void session.signOutAccount()} />
            </div>
          ) : null}
        </main>
      </div>
    </div>
  );
}
