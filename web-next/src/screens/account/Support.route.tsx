import {useEffect} from 'react';
import {defineWebScreen} from '../../route-registry';
import {router} from '../../router';

/** The drawn Help action is an external destination, not a new in-app page. */
function HelpScreen() {useEffect(() => {location.replace('https://getportico.tv/docs/');}, []); return null;}
/** Server-local account controls are the existing Profile & account section. */
function AccountScreen() {useEffect(() => {void router.navigate({to: '/settings/profile', replace: true});}, []); return null;}

export default [defineWebScreen({id: 'help', component: HelpScreen, chrome: 'standalone'}), defineWebScreen({id: 'account', component: AccountScreen, chrome: 'standalone'})] as const;
