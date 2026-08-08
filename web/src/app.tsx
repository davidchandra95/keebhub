import {Theme} from '@astryxdesign/core/theme';
import {neutralTheme} from '@astryxdesign/theme-neutral';
import {BrowserRouter, Route, Routes} from 'react-router-dom';

import {HealthPage} from './pages/HealthPage';
import {LoginPage} from './pages/LoginPage';
import {NotFoundPage} from './pages/NotFoundPage';

export function AppRoutes() {
  return (
    <Theme theme={neutralTheme} mode="light">
      <Routes>
        <Route path="/" element={<HealthPage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Theme>
  );
}

export function App() {
  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  );
}
