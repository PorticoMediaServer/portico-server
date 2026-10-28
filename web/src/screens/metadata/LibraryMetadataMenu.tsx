import {useState} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';
import {Menu} from '../../kit/overlays/Popover';

/** The library's quiet owner entry point. The caller supplies its current online-agent offer. */
export function LibraryMetadataMenu({onUnmatched}: {onUnmatched: () => void}) {
  const [open, setOpen] = useState(false), t = defaultI18n.t;
  return <Menu open={open} onOpenChange={setOpen} title={t('next.browse.libraryOptions')}
    trigger={<Button variant="icon" size="compact" icon="i-more" iconOnly aria-label={t('next.browse.libraryOptions')}/>}
    items={[
      {id: 'server', label: t('next.browse.server'), section: true, ownerOnly: true},
      {id: 'unmatched', label: t('next.ownerLibraries.tab.attention'), icon: 'i-list', onSelect: () => {setOpen(false); onUnmatched();}},
    ]}/>;
}
