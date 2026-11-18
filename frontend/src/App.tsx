import { useEffect, useState } from 'react';
import * as Service from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service';
import {
  Command,
  type Snapshot,
  type Status,
} from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/models';
import { AddDownload } from './AddDownload';
import { SettingsForm } from './SettingsForm';
import { JobList } from './JobList';
export function App() {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [info, setInfo] = useState<Status | null>(null);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [filter, setFilter] = useState('All downloads');
  const [search, setSearch] = useState('');
  const [settings, setSettings] = useState(false);
  const refresh = async () => {
    try {
      setSnapshot(await Service.Downloads());
    } catch {
      setError('Desktop backend disconnected. Restart the application.');
    }
  };
  useEffect(() => {
    let active = true;
    let timer: number;
    const poll = async () => {
      try {
        const [result, status] = await Promise.all([
          Service.Downloads(),
          Service.Info(),
        ]);
        if (active) {
          setSnapshot(result);
          setInfo(status);
          setError('');
        }
      } catch {
        if (active)
          setError('Desktop backend disconnected. Restart the application.');
      } finally {
        if (active)
          timer = window.setTimeout(() => {
            void poll();
          }, 250);
      }
    };
    void poll();
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, []);
  const control = async (c: Partial<Command>) => {
    try {
      await Service.Control(new Command(c));
      setMessage('Command accepted.');
      await refresh();
    } catch {
      setMessage(
        'Command failed. Pause all first, check the job status, then retry.',
      );
    }
  };
  const jobs = snapshot?.jobs ?? [];
  const visible = jobs.filter(
    (j) =>
      (filter === 'All downloads' ||
        (filter === 'History'
          ? j.state === 'completed' || j.state === 'cancelled'
          : j.state !== 'completed' && j.state !== 'cancelled')) &&
      `${j.name} ${j.host} ${j.destination}`
        .toLowerCase()
        .includes(search.toLowerCase()),
  );
  return (
    <main>
      <header>
        <div>
          <p className="eyebrow">IDM Enhanced · desktop alpha</p>
          <h1>Your downloads, under control.</h1>
          <p>Local downloads with safe recovery.</p>
        </div>
        <button
          onClick={() => {
            void Service.Quit().catch(() =>
              setMessage('Unable to quit. Try the tray menu.'),
            );
          }}
        >
          Quit safely
        </button>
      </header>
      <p className="lifecycle">
        {info?.lifecycle} Quit safely pauses active downloads. After sleep,
        interrupted jobs stay paused.
      </p>
      <p role="alert">{error}</p>
      <p role="status" aria-label="Download session status">
        {snapshot?.message ||
          (snapshot?.sleeping
            ? 'Downloads paused for sleep.'
            : snapshot?.running
              ? 'Downloads running'
              : 'Ready')}
      </p>
      <p role="status" aria-label="Command feedback">
        {message}
      </p>
      {info?.notification && <p role="status">{info.notification}</p>}
      <div className="workspace">
        <aside>
          <nav aria-label="Download filters">
            {['All downloads', 'Active & queued', 'History'].map((label) => (
              <button
                key={label}
                aria-pressed={!settings && filter === label}
                onClick={() => {
                  setFilter(label);
                  setSettings(false);
                }}
              >
                {label}
              </button>
            ))}
            <button
              aria-pressed={settings}
              onClick={() => setSettings(!settings)}
            >
              Settings
            </button>
          </nav>
          <details>
            <summary>Connection details</summary>
            <p>{info?.platform ?? 'Connecting…'}</p>
            <p>{info?.ipc}</p>
            <p>Last browser ping: {info?.lastPing || 'None'}</p>
            <button
              onClick={() => {
                void Service.Hello()
                  .then(setMessage)
                  .catch(() => setMessage('Backend connection failed.'));
              }}
            >
              Test backend connection
            </button>
          </details>
        </aside>
        <div>
          {settings && snapshot ? (
            <SettingsForm
              settings={snapshot.settings}
              saved={() => {
                void refresh();
              }}
            />
          ) : (
            <>
              <AddDownload
                added={() => {
                  void refresh();
                }}
              />
              <section aria-labelledby="list-title">
                <div className="toolbar">
                  <h2 id="list-title">{filter}</h2>
                  <button
                    className="primary"
                    disabled={!snapshot || snapshot.running || !!error}
                    onClick={() => {
                      void control({ action: 'run-queue' });
                    }}
                  >
                    Run queue
                  </button>
                  <button
                    disabled={!snapshot?.running}
                    onClick={() => {
                      void control({ action: 'pause-all' });
                    }}
                  >
                    Pause all
                  </button>
                </div>
                <label>
                  Search downloads
                  <input
                    type="search"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Name, folder or host"
                  />
                </label>
                <JobList
                  jobs={visible}
                  allJobs={jobs}
                  running={snapshot?.running ?? false}
                  control={control}
                />
              </section>
            </>
          )}
        </div>
      </div>
    </main>
  );
}
