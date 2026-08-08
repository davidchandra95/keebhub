import {StrictMode} from 'react';
import {createRoot} from 'react-dom/client';

import '@astryxdesign/core/reset.css';
import '@astryxdesign/core/astryx.css';
import '@astryxdesign/theme-neutral/theme.css';
import './styles.css';

import {App} from './app';

const root = document.getElementById('root');
if (root === null) {
  throw new Error('root element is missing');
}

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
