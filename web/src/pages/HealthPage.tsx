import {useCallback, useEffect, useState} from 'react';

import {AppButton} from '../components/AppButton';
import {AppCard} from '../components/AppCard';

type HealthState = 'checking' | 'connected' | 'unavailable';

export function HealthPage() {
  const [health, setHealth] = useState<HealthState>('checking');

  const checkHealth = useCallback(async () => {
    setHealth('checking');

    try {
      const response = await fetch('/healthz', {headers: {Accept: 'application/json'}});
      if (!response.ok) {
        throw new Error(`health check returned ${response.status}`);
      }
      setHealth('connected');
    } catch {
      setHealth('unavailable');
    }
  }, []);

  useEffect(() => {
    void checkHealth();
  }, [checkHealth]);

  return (
    <main className="shell">
      <section className="hero" aria-labelledby="page-title">
        <p className="eyebrow">Keyboard marketplace</p>
        <h1 id="page-title">KeebHub</h1>
        <p className="intro">
          The application foundation is ready. Product features will arrive in focused,
          tested slices.
        </p>
      </section>

      <AppCard>
        <div className="status-heading">
          <div>
            <p className="card-label">System status</p>
            <h2>Backend connection</h2>
          </div>
          <span className={`status-dot status-dot--${health}`} aria-hidden="true" />
        </div>

        <div className="status-copy" aria-live="polite">
          {health === 'checking' && (
            <>
              <strong>Checking backend</strong>
              <p>Waiting for the health endpoint to respond.</p>
            </>
          )}
          {health === 'connected' && (
            <>
              <strong>Backend connected</strong>
              <p>The KeebHub service is running and can accept requests.</p>
            </>
          )}
          {health === 'unavailable' && (
            <>
              <strong>Backend unavailable</strong>
              <p>Start the API service, then try the health check again.</p>
              <AppButton label="Retry" onClick={() => void checkHealth()} />
            </>
          )}
        </div>
      </AppCard>
    </main>
  );
}
