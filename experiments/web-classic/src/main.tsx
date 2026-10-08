import React from 'react';
import {createRoot} from 'react-dom/client';
import {RouterProvider} from '@tanstack/react-router';
import './styles/tokens.css';
import './styles/base.css';
import {readDevFlags} from './app/devflags';
import {router} from './app/router';

readDevFlags();

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>,
);
