import {playOnDestinations, webDestinationSupport} from '@core/presentation/index.ts';
import {IconButton, Menu, type MenuItem} from '../ui';
import {useI18n} from '../app/i18n';

type AirPlayVideo = HTMLVideoElement & {webkitShowPlaybackTargetPicker?: () => void};

/**
 * "Play on" (X-18, Justin's simplification): Google Cast where the browser has
 * the Cast SDK (Chromium) and AirPlay through Safari's system picker on macOS.
 * Portico devices come only from local-network discovery, which browsers
 * can't do, so web shows none; never a remote device list or a TV code.
 */
export function PlayOnButton({castAvailable, onCast, video}: {castAvailable: boolean; onCast: () => void; video: HTMLVideoElement | null}) {
  const i18n = useI18n();
  const airplay = typeof window !== 'undefined' && 'WebKitPlaybackTargetAvailabilityEvent' in window && !!(video as AirPlayVideo | null)?.webkitShowPlaybackTargetPicker;
  const support = webDestinationSupport({chromium: typeof window !== 'undefined' && 'chrome' in window, castApi: castAvailable, safariAirPlay: airplay});
  // Browsers can't run local-network discovery, so web has no nearby Portico devices of its own.
  const rows = playOnDestinations('web', support).rows;
  // Nothing to play on: no button at all (no dead list).
  if (!rows.length) return null;
  const items: MenuItem[] = rows.map(r => (r.kind === 'porticoDevice' ? {id: `device:${r.id}`, label: r.name, icon: r.icon} : {id: r.kind, label: i18n.t(r.label), icon: r.icon, ...(r.kind === 'airplay' ? {meta: i18n.t(r.subtitle)} : {})}));
  return (
    <Menu
      label={i18n.t('player.playOn')}
      trigger={<IconButton name="googleCast" label={i18n.t('player.playOn')} variant="ghost" />}
      items={items}
      onSelect={id => {
        if (id === 'googleCast') onCast();
        else if (id === 'airplay') (video as AirPlayVideo | null)?.webkitShowPlaybackTargetPicker?.();
      }}
    />
  );
}
