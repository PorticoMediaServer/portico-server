import {useEffect, useRef, useState} from 'react';
import {defineWebScreen} from '../../route-registry';
import {useWebScreenContext} from '../../route-registry-view';
import {createBrowserAccountEntry, type BrowserAccountEntry} from '../../platform/account-entry';
import {takeLocalMoveContinuation} from '../../platform/local-move-continuation';
import {takeLocalMove} from '../../platform/local-move';
import {router} from '../../router';
import {AccountSignInForm} from './sign-in-form';
import {AccountSignInView} from './AccountSignInView';

function SignInScreen() {
  const {takeAccountReturn, takeAccountMove} = useWebScreenContext();
  const [instance, setInstance] = useState<Readonly<{entry: BrowserAccountEntry; form: AccountSignInForm; continuation: boolean}>>();
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    // An owner move to local hands over one private entry instance so this screen
    // renders its real challenge/profile/ready states; the one-shot custody is
    // admitted here, by that entry, never in the retiring Settings view. Anything
    // else is the ordinary per-mount sign-in, and any stranded handoff is released.
    const continuation = takeLocalMoveContinuation(), custody = takeLocalMove();
    if (continuation && custody) {
      const owned = continuation;
      setInstance({...owned, continuation: true});
      void owned.entry.completeLocalOwnerMove(custody).then(admitted => {
        // A stale grant was refused and retired; show the server the app really has.
        if (!admitted && alive.current) void router.navigate({to: '/servers', replace: true});
      }).catch(() => {if (alive.current) void router.navigate({to: '/servers', replace: true});});
      return () => {alive.current = false; owned.form.dispose(); owned.entry.dispose();};
    }
    if (continuation) {continuation.form.dispose(); continuation.entry.dispose();}
    if (custody) void custody.retire();
    const entry = createBrowserAccountEntry(), owned = {entry, form: new AccountSignInForm(entry)};
    setInstance({...owned, continuation: false});
    return () => {alive.current = false; owned.form.dispose(); owned.entry.dispose();};
  }, []);
  return instance ? <AccountSignInView entry={instance.entry} form={instance.form} initialize={!instance.continuation} takeReturn={takeAccountReturn} takeMove={takeAccountMove} /> : null;
}
export default defineWebScreen({id: 'signIn', component: SignInScreen, chrome: 'standalone'});
