import { useState } from 'react';
import { Command } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/models';
import type { Settings } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager/models';
import * as Service from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service';
export function SettingsForm({
  settings,
  saved,
}: {
  settings: Settings;
  saved: () => void;
}) {
  const [draft, setDraft] = useState(settings);
  const [message, setMessage] = useState('');
  const save = async () => {
    try {
      await Service.SaveSettings(draft);
      setMessage('Settings saved.');
      saved();
    } catch {
      setMessage(
        'Could not save settings. Pause all downloads and check the limits.',
      );
    }
  };
  return (
    <section aria-labelledby="settings-title">
      <h2 id="settings-title">Download settings</h2>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void save();
        }}
      >
        {(
          [
            ['workers', 'Range workers', 1, 16],
            ['jobs', 'Simultaneous jobs', 1, 16],
            ['perHost', 'Requests per host', 1, 64],
            ['retries', 'Automatic retries', 0, 8],
            ['rate', 'Bandwidth (bytes/sec, 0 = unlimited)', 0, 1099511627776],
          ] as const
        ).map(([key, label, min, max]) => (
          <label key={key}>
            {label}
            <input
              type="number"
              required
              min={min}
              max={max}
              value={draft[key]}
              onChange={(e) =>
                setDraft({ ...draft, [key]: e.target.valueAsNumber })
              }
            />
          </label>
        ))}
        <label className="checkbox">
          <input
            type="checkbox"
            checked={draft.notifications}
            onChange={(e) =>
              setDraft({ ...draft, notifications: e.target.checked })
            }
          />
          System completion notifications
        </label>
        <button
          type="button"
          onClick={() => {
            void Service.Control(
              new Command({ action: 'authorize-notifications' }),
            )
              .then(() => setMessage('Notification permission granted.'))
              .catch(() =>
                setMessage(
                  'System notifications unavailable. Completion remains in History.',
                ),
              );
          }}
        >
          Authorize system notifications
        </button>
        <button type="submit">Save settings</button>
        <p role="status">{message}</p>
      </form>
    </section>
  );
}
