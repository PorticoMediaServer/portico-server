import {unconfigured, useRead, type Read} from '../../admin/console';
import {lookupStatus, lookupStatusId, parseScreenLookup, type ScreenLookupPolicy} from '../../admin/metadata-screen';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Badge, Button, SettingsRow} from '../../ui';

const providerNames: Record<string, string> = {tmdb: 'TMDB', tvdb: 'TVDB', anilist: 'AniList'};
export const providerList = (p: ScreenLookupPolicy) => p.providers.map(x => providerNames[x] ?? x.toUpperCase()).join(', ');

/** The library's film/TV lookup policy; null for libraries without one (music, audiobooks). */
export function useScreenLookup(libraryId: string, deps: readonly unknown[] = []): Read<ScreenLookupPolicy | null> {
  const {api} = useSession();
  return useRead(async () => {
    const raw = await unconfigured(api.request<unknown>(`/v1/libraries/${encodeURIComponent(libraryId)}/metadata/screen`));
    return raw === null ? null : parseScreenLookup(raw);
  }, [api, libraryId, ...deps]);
}

/** Library page: where title lookups stand, in plain words, with a way into the setting. */
export function LookupStatusRow({policy, onChange}: {policy: ScreenLookupPolicy; onChange: () => void}) {
  const {t} = useI18n();
  const status = lookupStatus(policy);
  const tone = status === 'on' ? 'healthy' : status === 'off' ? 'neutral' : 'warning';
  // The metadata source decides: local only says so in its own words.
  const local = policy.agent === 'local';
  return (
    <SettingsRow
      label={t('web.metadataSource.title')}
      help={local ? t('web.metadataSource.statusLocal') : <>{status === 'on' ? t('web.metadataSource.statusOnline', {providers: providerList(policy)}) : t(lookupStatusId(status) as never, {providers: providerList(policy)})}{status === 'needs_consent' ? <><br />{t('web.metadataLookup.turnOnHint')}</> : null}</>}
      meta={local ? undefined : <Badge tone={tone} dot>{t(status === 'on' ? 'web.metadataLookup.badge.on' : status === 'off' ? 'web.metadataLookup.badge.off' : 'web.metadataLookup.badge.waiting')}</Badge>}
      control={<Button size="sm" variant="outline" label={t('web.metadataLookup.change')} onClick={onChange} />}
    />
  );
}
