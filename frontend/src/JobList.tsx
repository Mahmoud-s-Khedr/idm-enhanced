import { useState } from 'react';
import type {
  JobView,
  Command,
} from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/models';
function bytes(n: number) {
  return `${(n / 1048576).toFixed(2)} MiB`;
}
export function JobList({
  jobs,
  allJobs,
  running,
  control,
}: {
  jobs: JobView[];
  allJobs: JobView[];
  running: boolean;
  control: (c: Partial<Command>) => Promise<void>;
}) {
  const [cancelID, setCancelID] = useState('');
  const reorder = (id: string) => {
    const order = allJobs.filter((j) => j.state === 'queued').map((j) => j.id);
    void control({
      action: 'reorder',
      order: [id, ...order.filter((value) => value !== id)],
    });
  };
  return (
    <div className="job-list" aria-label="Downloads">
      {jobs.length === 0 && (
        <p className="empty">
          No downloads here. Add a URL or change your filter.
        </p>
      )}
      {jobs.map((j) => (
        <article key={j.id} aria-label={j.name}>
          <div className="job-heading">
            <h3>{j.name}</h3>
            <span className={`badge ${j.state}`}>
              {j.state.replaceAll('_', ' ')}
            </span>
          </div>
          <p className="hint">
            {j.host} · {j.destination}
          </p>
          <progress
            aria-label={`Progress for ${j.name}`}
            value={j.total > 0 ? j.bytes : undefined}
            max={j.total > 0 ? j.total : undefined}
          />
          <p>
            {bytes(j.bytes)}
            {j.total >= 0 ? ` of ${bytes(j.total)}` : ' · size unknown'}
          </p>
          {j.scheduled && (
            <p>Scheduled: {new Date(j.scheduled).toLocaleString()}</p>
          )}
          {j.retryAt && (
            <p>Retry after: {new Date(j.retryAt).toLocaleString()}</p>
          )}
          {j.error && (
            <p role="status">Attention: {j.error.replaceAll('_', ' ')}</p>
          )}
          <div className="actions">
            {j.state === 'completed' ? (
              <>
                <button
                  onClick={() => {
                    void control({ action: 'open', id: j.id });
                  }}
                >
                  Open file
                </button>
                <button
                  onClick={() => {
                    void control({ action: 'reveal', id: j.id });
                  }}
                >
                  Show in folder
                </button>
                <button
                  disabled={running}
                  onClick={() => {
                    void control({ action: 'remove', id: j.id });
                  }}
                >
                  Remove history
                </button>
              </>
            ) : (
              <>
                <button
                  disabled={running}
                  onClick={() => {
                    void control({ action: 'resume', id: j.id });
                  }}
                >
                  Resume / retry
                </button>
                <button
                  onClick={() => {
                    void control({ action: 'pause', id: j.id });
                  }}
                >
                  Pause
                </button>
                <button
                  disabled={running}
                  onClick={() => {
                    if (
                      window.confirm(
                        'Discard this job’s saved progress and download again?',
                      )
                    )
                      void control({ action: 'restart', id: j.id });
                  }}
                >
                  Restart
                </button>
                <button disabled={running} onClick={() => setCancelID(j.id)}>
                  Cancel…
                </button>
                {j.state === 'queued' && (
                  <>
                    <button disabled={running} onClick={() => reorder(j.id)}>
                      Move to front
                    </button>
                    {j.scheduled && (
                      <button
                        disabled={running}
                        onClick={() => {
                          void control({
                            action: 'schedule',
                            id: j.id,
                            at: '',
                          });
                        }}
                      >
                        Clear schedule
                      </button>
                    )}
                  </>
                )}
                {j.state === 'cancelled' && (
                  <button
                    disabled={running}
                    onClick={() => {
                      void control({ action: 'remove', id: j.id });
                    }}
                  >
                    Remove history
                  </button>
                )}
              </>
            )}
          </div>
          {cancelID === j.id && (
            <div
              className="retention"
              role="group"
              aria-label="Partial file retention"
            >
              <p>Keep the partial file for a later resume, or discard it?</p>
              <button
                onClick={() => {
                  void control({
                    action: 'cancel',
                    id: j.id,
                    keepPartial: true,
                  });
                  setCancelID('');
                }}
              >
                Keep partial
              </button>
              <button
                onClick={() => {
                  void control({
                    action: 'cancel',
                    id: j.id,
                    keepPartial: false,
                  });
                  setCancelID('');
                }}
              >
                Discard partial
              </button>
              <button onClick={() => setCancelID('')}>Go back</button>
            </div>
          )}
        </article>
      ))}
    </div>
  );
}
