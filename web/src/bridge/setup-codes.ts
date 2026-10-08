import {type SetupDraft} from '@core/setup-onboarding';
import {BrowserProtectedStore} from './server-connections';
export const browserSetupDraft=(serverId:string)=>new BrowserProtectedStore<SetupDraft>('portico.initialize.'+serverId);
