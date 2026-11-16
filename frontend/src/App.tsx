import { useEffect, useState } from 'react';
import * as Service from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service';
import type { Status } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/models';

export function App() {
  const [info, setInfo] = useState<Status | null>(null);
  const [error, setError] = useState('');
  const [response, setResponse] = useState('');
  useEffect(() => {
    let active = true;
    const refresh = async () => {
      try {
        const result = await Service.Info();
        if (active) {
          setInfo(result);
          setError('');
        }
      } catch {
        if (active) {
          setInfo(null);
          setError('Desktop backend disconnected. Restart the application.');
        }
      }
    };
    void refresh();
    const timer = window.setInterval(() => {
      void refresh();
    }, 1000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, []);
  const hello = async () => {
    try {
      setResponse(await Service.Hello());
    } catch {
      setResponse('Backend call failed. Restart the application.');
    }
  };
  return (
    <main>
      <p className="eyebrow">IDM Enhanced · M0</p>
      <h1>A connected foundation.</h1>
      <p>Desktop and browser platform spike</p>
      <section aria-labelledby="backend-title">
        <h2 id="backend-title">Desktop backend</h2>
        <p role="status">{error || (info ? 'Connected' : 'Connecting…')}</p>
        <dl>
          <dt>Build</dt>
          <dd>{info?.build ?? 'Waiting for backend'}</dd>
          <dt>Platform</dt>
          <dd>{info?.platform ?? 'Unknown'}</dd>
        </dl>
        <button
          onClick={() => {
            void hello();
          }}
        >
          Test backend connection
        </button>
        <p aria-live="polite">{response}</p>
      </section>
      <section aria-labelledby="browser-title">
        <h2 id="browser-title">Browser connection</h2>
        <p>{info?.ipc ?? 'Desktop disconnected'}</p>
        <p>
          Last acknowledged ping:{' '}
          <strong>{info?.lastPing || 'No browser connected yet'}</strong>
        </p>
        <p>
          Open the development extension and select “Test desktop connection”.
        </p>
      </section>
    </main>
  );
}
