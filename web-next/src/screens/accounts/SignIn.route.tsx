import {useEffect, useState} from 'react';
import {defineWebScreen} from '../../route-registry';
import {useWebScreenContext} from '../../route-registry-view';
import {createBrowserAccountEntry, type BrowserAccountEntry} from '../../platform/account-entry';
import {AccountSignInForm} from './sign-in-form';
import {AccountSignInView} from './AccountSignInView';

function SignInScreen() {
  const {takeAccountReturn, takeAccountMove} = useWebScreenContext();
  const [instance, setInstance] = useState<Readonly<{entry: BrowserAccountEntry; form: AccountSignInForm}>>();
  useEffect(() => {
    const entry = createBrowserAccountEntry(), form = new AccountSignInForm(entry);
    setInstance({entry, form});
    return () => {form.dispose(); entry.dispose();};
  }, []);
  return instance ? <AccountSignInView {...instance} takeReturn={takeAccountReturn} takeMove={takeAccountMove} /> : null;
}
export default defineWebScreen({id: 'signIn', component: SignInScreen, chrome: 'standalone'});
