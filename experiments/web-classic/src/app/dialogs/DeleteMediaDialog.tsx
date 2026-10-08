import {useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient} from '@/data/DataProvider';
import type {Entry} from '@/data/types';
import {ConfirmDialog, useToast} from '@/ui/Overlay';
import {Checkbox} from '@/ui/Form';
import {plural} from '@/app/format';
import {kindNoun} from '@/app/links';

/** Deletes media from the server. Files can go to the server's deleted-titles bin, where they are kept for 30 days. */
export default function DeleteMediaDialog({entries, onClose}: {entries: Entry[]; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const navigate = useNavigate();
  const [removeFiles, setRemoveFiles] = useState(true);
  const one = entries.length === 1 ? entries[0]! : undefined;
  const what = one ? `“${one.title}”` : plural(entries.length, 'item');
  return (
    <ConfirmDialog
      open
      onOpenChange={o => !o && onClose()}
      title={one ? `Delete ${one.title}?` : `Delete ${plural(entries.length, 'item')}?`}
      body={<>This removes {what} from the library for everyone on this server{one?.kind === 'show' ? ', with all its seasons and episodes' : ''}. Watch history and ratings for {one ? `this ${kindNoun(one)}` : 'them'} are removed too.</>}
      action="Delete"
      tone="danger"
      typeToConfirm="delete"
      onConfirm={async () => {
        await client.catalog.deleteMedia(entries.map(e => e.id), removeFiles);
        toast.show(`Deleted ${what}`, {icon: 'trash'});
        onClose();
        if (one && location.pathname.includes(encodeURIComponent(one.id))) void navigate({to: '/' as never});
      }}
    >
      <div style={{marginTop: 14}}>
        <Checkbox checked={removeFiles} onChange={setRemoveFiles} label="Also move the files to deleted titles" help="Deleted files are kept for 30 days under Manage server › Maintenance, then removed from disk. Leave this off to keep the files where they are; the next scan will skip them." />
      </div>
    </ConfirmDialog>
  );
}
