// Real ResetPage form/controller. The synthetic unsubmitted token only selects its form view;
// there is no account fixture, API interception, successful reset or adopted session here.
import {createRoot} from 'react-dom/client';
import {IconSprite} from '../../src/kit/Icon';
import {ResetPage} from '../../src/services/LinkPages';
import {servicesAccount} from '../../src/services/session';
import '../../src/kit/tokens.css';
import '../../src/kit/fonts.css';
import '../../src/bootstrap.css';

createRoot(document.getElementById('root')!).render(<><IconSprite /><ResetPage account={servicesAccount()} token="unsubmitted-layout-test" /></>);
