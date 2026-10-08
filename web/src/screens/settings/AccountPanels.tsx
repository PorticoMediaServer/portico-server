import {useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {friendlyHost, type SettingsCustomId} from '@core/presentation/index.ts';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {useServerPreferences} from '../../app/server-preferences';
import {Button, ConfirmDialog, Notice, SettingsGroup, SettingsRow, Surface, Text} from '../../ui';
import {DeviceNameRow, DevicesSection, ProfileRestrictionsDialog, TwoFactorSection} from './Security';
import {LocalProfiles, PasswordGroup} from './Profiles';
import s from './Settings.module.css';

/** Panels that bring their own group and heading; the rest are rows inside their section's group. */
const GROUP_PANELS: ReadonlySet<SettingsCustomId> = new Set(['identity', 'profiles', 'password', 'twoStep', 'devices', 'porticoAccount', 'about']);
export const isGroupPanel = (id: SettingsCustomId) => GROUP_PANELS.has(id);

/** The account pages' `custom` rows (settings-structure.ts), drawn with the web's own panels. */
export function AccountPanel({id}: {id: SettingsCustomId}) {
  switch (id) {
    case 'identity': return <Identity />;
    case 'profiles': return <Profiles />;
    case 'password': return <PasswordGroup />;
    case 'twoStep': return <TwoFactorSection />;
    case 'devices': return <DevicesSection />;
    case 'porticoAccount': return <PorticoAccount />;
    case 'servers': return <ConnectedServer />;
    case 'subtitlePreview': return <SubtitlePreview />;
    case 'deviceName': return <DeviceNameRow />;
    case 'about': return <About />;
    default: return null;
  }
}

function Identity() {
  const session = useSession();
  const {t} = useI18n();
  const hosted = session.hosted;
  const profileId = session.session?.viewer.profileId;
  const profile = hosted?.profiles.find(p => p.id === profileId)?.name ?? session.local?.profiles.find(p => p.id === profileId)?.name;
  return (
    <Surface>
      <Text as="p" variant="label" tone="tertiary">{t('account.signedInAs')}</Text>
      <Text as="p" variant="heading" className={s.identityName}>{hosted?.account.displayName ?? hosted?.account.username ?? session.local?.username ?? t('web.account.fallbackName')}</Text>
      {profile ? <Text as="p" variant="caption" tone="secondary">{t('web.account.profileLine', {name: profile})}</Text> : null}
    </Surface>
  );
}

/** Direct-sign-in servers manage profiles here (name, picture, PIN, limits); a Portico Account lists its own. */
function Profiles() {
  const session = useSession();
  const {t} = useI18n();
  const [limits, setLimits] = useState<{id: string; name: string}>();
  if (session.session?.viewer.authority === 'local') return <><LocalProfiles onLimits={setLimits} />{limits ? <ProfileRestrictionsDialog profile={limits} onClose={() => setLimits(undefined)} /> : null}</>;
  if (!session.hosted?.profiles.length) return null;
  return (
    <SettingsGroup title={t('profiles.title')} description={t('web.account.profilesHostedHelp')}>
      {session.hosted.profiles.map(p => <SettingsRow key={p.id} icon="profile" label={p.name} meta={p.id === session.session?.viewer.profileId ? t('profile.current') : undefined} />)}
    </SettingsGroup>
  );
}

/** Password and two-step verification of a Portico Account are the account's, on the account site. */
function PorticoAccount() {
  const session = useSession();
  const {t} = useI18n();
  const navigate = useNavigate();
  const member = session.session?.viewer.authority === 'local' && !!session.local?.hostedAccountId;
  // An account that lives only on this server has no Portico Account to manage: its password, two-step and devices are the rows above.
  if (!member && !session.hosted) return null;
  return <Notice tone="info" title={member ? t('web.account.porticoMemberTitle') : t('account.manageHosted')} action={{label: t('web.account.openAccount'), onClick: () => void navigate({to: '/account', search: {}})}}>{member ? t('web.account.porticoMemberBody') : t('web.account.manageHostedBody')}</Notice>;
}

function ConnectedServer() {
  const session = useSession();
  const {t} = useI18n();
  const [disconnect, setDisconnect] = useState(false);
  const hosted = session.session?.viewer.authority === 'hosted';
  return (
    <>
      <SettingsRow icon="server" label={session.system?.name ?? t('web.servers.fallbackName')} help={[hosted ? t('web.settings.throughAccount') : t('web.settings.direct'), session.system?.version ? t('web.servers.serverVersion', {version: session.system.version}) : '', friendlyHost(session.serverUrl) ?? ''].filter(Boolean).join(' · ')}
        control={<Button size="sm" variant="outline" label={t('web.servers.disconnect')} onClick={() => setDisconnect(true)} />} />
      <ConfirmDialog open={disconnect} onOpenChange={setDisconnect} title={t('web.servers.disconnectTitle')} body={t('web.servers.disconnectBody')} confirmLabel={t('web.servers.disconnectAction')} cancelLabel={t('action.cancel')} destructive={false} onConfirm={() => { setDisconnect(false); session.disconnectServer(); }} />
    </>
  );
}

/** The chosen size and background on a line of text, as the player draws them (`Player.module.css`). */
function SubtitlePreview() {
  const {t} = useI18n();
  const server = useServerPreferences();
  return (
    <div className={s.preview} data-size={server.value<string>('playback.subtitleSize', 'medium')} data-background={server.value<string>('playback.subtitleBackground', 'translucent')} aria-hidden>
      <span className={s.previewLine}>{t('settings.subtitlePreview.line')}</span>
    </div>
  );
}

function About() {
  const session = useSession();
  const {t} = useI18n();
  // WEB-SET-03: no codename, and the developer gallery only in development builds.
  const version = (import.meta.env.VITE_APP_VERSION as string | undefined) || '0.1.0';
  return (
    <>
      <SettingsGroup>
        <SettingsRow label={t('web.about.webApp')} meta={version} />
        <SettingsRow label={t('web.about.server')} meta={session.system?.name ?? '—'} />
        <SettingsRow label={t('web.about.serverVersion')} meta={session.system?.version ?? '—'} />
      </SettingsGroup>
      <SettingsGroup>
        <SettingsRow icon="external" label={t('web.about.help')} onClick={() => window.open('https://getportico.tv/help', '_blank', 'noopener')} />
        <SettingsRow icon="external" label={t('web.about.terms')} onClick={() => window.open('https://getportico.tv/terms', '_blank', 'noopener')} />
        <SettingsRow icon="external" label={t('web.about.privacy')} onClick={() => window.open('https://getportico.tv/privacy', '_blank', 'noopener')} />
      </SettingsGroup>
      <div className={s.legal}>
        <img src="/assets/tmdb.svg" alt="TMDB" height={16} className={s.legalMark} />
        <Text as="p" variant="caption" tone="tertiary">{t('web.about.legal')} {t('web.about.tmdb')}</Text>
      </div>
      <Text as="p" variant="caption" tone="tertiary">{t('about.datasetCredit')}</Text>
      {import.meta.env.DEV ? <div><Button variant="ghost" size="sm" label={t('settings.designGallery')} onClick={() => window.open('/gallery', '_blank')} /></div> : null}
    </>
  );
}
