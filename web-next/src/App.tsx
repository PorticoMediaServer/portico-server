import {RouterProvider} from '@tanstack/react-router';
import {IconSprite} from './kit/Icon';
import {router} from './router';
export function App() {
  return <><IconSprite /><RouterProvider router={router} /></>;
}
