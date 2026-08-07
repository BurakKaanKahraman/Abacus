import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { App } from './App';
import { resolveInitialTheme } from './hooks/useTheme';
import './styles/global.css';

// Applied before the first render so a stored light theme does not flash the
// dark palette while React mounts.
document.documentElement.dataset.theme = resolveInitialTheme();

const container = document.getElementById('root');
if (!container) {
  throw new Error('Root container #root is missing from index.html');
}

createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
