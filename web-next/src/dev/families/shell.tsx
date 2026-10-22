import {defineGalleryFamily} from '../gallery-registry';
import {createI18n} from '@i18n';
import {shellFixtures} from '../../../../packages/i18n/src/catalog/en-US/next/dev/shell-fixtures';
import {OwnerTag, ServerBand, ServerHeader, ServerMark} from '../../kit/shell';
import styles from '../../kit/shell/gallery.module.css';

const t = createI18n().t;
const facts = [
  {id: 'uptime', text: shellFixtures['next.dev.shell.uptime'], role: 'supplement' as const},
  {id: 'storage', text: shellFixtures['next.dev.shell.storage'], role: 'supplement' as const},
  {id: 'database', text: shellFixtures['next.dev.shell.database'], role: 'supplement' as const},
];
function OwnerState() { return <div className={styles.stateStage}><ServerBand variant="state" /><div className={styles.stateIdentity}><OwnerTag variant="state" identity={t('next.shell.serverSettings')} /></div></div>; }
function OwnerDetails() { return <div className={styles.detailStage}><div className={styles.detailHeading}><ServerMark />{t('next.shell.forOwner')}<OwnerTag /></div></div>; }
function CompactOwner() { return <div className={styles.compactStage}><div className={styles.compactHeading}><ServerMark size={22} />{t('next.shell.forOwner')}<OwnerTag /></div></div>; }
function HeaderOwner() { return <div className={styles.headerStage}><ServerBand /><div className={styles.headerPosition}><ServerHeader serverName={shellFixtures['next.dev.shell.serverName']} viewerRole="owner" audience="owner" health="normal" facts={facts} /></div></div>; }
function HeaderShared() { return <div className={styles.headerStage}><ServerBand /><div className={styles.headerPosition}><ServerHeader serverName={shellFixtures['next.dev.shell.serverName']} viewerRole="owner" audience="administrators" health="normal" facts={facts} /></div></div>; }
function HeaderAdministrator() { return <div className={styles.headerStage}><ServerBand /><div className={styles.headerPosition}><ServerHeader serverName={shellFixtures['next.dev.shell.serverName']} viewerRole="administrator" audience="administrators" health="attention" facts={facts} /></div></div>; }

export default defineGalleryFamily({id: 'shell', labelKey: 'next.shell.galleryTitle', cases: [
  {id: 'owner-state', labelKey: 'next.shell.case.ownerState', width: 390, height: 300, render: OwnerState},
  {id: 'owner-details', labelKey: 'next.shell.case.ownerDetails', width: 620, height: 62, render: OwnerDetails},
  {id: 'owner-compact', labelKey: 'next.shell.case.ownerCompact', width: 354, height: 54, render: CompactOwner},
  {id: 'server-header-owner', labelKey: 'next.shell.case.serverHeader', width: 1440, height: 420, render: HeaderOwner},
  {id: 'server-header-shared-owner', labelKey: 'next.shell.case.serverShared', width: 1440, height: 420, render: HeaderShared},
  {id: 'server-header-administrator', labelKey: 'next.shell.case.serverAdministrator', width: 1440, height: 420, render: HeaderAdministrator},
]});
