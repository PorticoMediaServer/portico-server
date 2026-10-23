import {defineWebScreen} from '../../route-registry';
import {ReadRouteEntry} from './ReadEntry';
import {readSearch} from './route-search';

export default defineWebScreen({id: 'read', component: ReadRouteEntry, chrome: 'standalone', validateSearch: readSearch});
