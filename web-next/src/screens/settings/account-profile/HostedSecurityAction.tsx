import {Button} from '../../../kit/controls';

/**
 * SET-4AA18AB: the action control on the account-profile Sign-in rows. The work
 * happens here, in the page's own canonical Hosted SecurityTask dialog, so this is
 * a plain glass/small button carrying only what the owner hands it — wording,
 * disabled state and one callback. No URL, navigation, identity lookup, API,
 * hidden request or click-side authority: the owner's callback opens the dialog it
 * guards, in place.
 */
export function HostedSecurityAction({label, disabled, onOpen}: {label: string; disabled: boolean; onOpen: () => void}) {
  return <Button size="small" variant="glass" disabled={disabled} onClick={onOpen}>{label}</Button>;
}
