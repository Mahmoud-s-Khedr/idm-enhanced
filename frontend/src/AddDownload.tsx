import { useState } from 'react';
import * as Service from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service';
import { DownloadRequest } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/models';

export function AddDownload({ added }: { added: () => void }) {
  const [url, setURL] = useState('');
  const [destination, setDestination] = useState('');
  const [checksum, setChecksum] = useState('');
  const [scheduled, setScheduled] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const submit = async () => {
    setBusy(true);
    try {
      await Service.Add(
        new DownloadRequest({
          url,
          destination,
          sha256: checksum,
          scheduled: scheduled ? new Date(scheduled).toISOString() : '',
        }),
      );
      setURL('');
      setDestination('');
      setChecksum('');
      setScheduled('');
      setMessage('Download added to the queue.');
      added();
    } catch {
      setMessage(
        'Could not add download. Check the HTTP URL, new destination and checksum.',
      );
    } finally {
      setBusy(false);
    }
  };
  const browse = async () => {
    try {
      const path = await Service.ChooseDestination();
      if (path) setDestination(path);
    } catch {
      setMessage('Destination chooser unavailable. Enter a full path.');
    }
  };
  return (
    <section aria-labelledby="add-title">
      <h2 id="add-title">Add download</h2>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <label>
          Source URL
          <input
            required
            type="url"
            value={url}
            placeholder="https://example.com/file.zip"
            onChange={(e) => setURL(e.target.value)}
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => {
              e.preventDefault();
              setURL(
                e.dataTransfer
                  .getData('text/uri-list')
                  .split('\n')
                  .find((line) => line && !line.startsWith('#')) ??
                  e.dataTransfer.getData('text/plain'),
              );
            }}
          />
        </label>
        <label>
          Save as
          <span className="input-row">
            <input
              required
              value={destination}
              onChange={(e) => setDestination(e.target.value)}
              placeholder="Full path to a new file"
            />
            <button
              type="button"
              onClick={() => {
                void browse();
              }}
            >
              Browse…
            </button>
          </span>
        </label>
        <details>
          <summary>Schedule and verification</summary>
          <label>
            Start at (local time)
            <input
              type="datetime-local"
              value={scheduled}
              onChange={(e) => setScheduled(e.target.value)}
            />
          </label>
          <p className="hint">
            One-time schedule in your current timezone (
            {Intl.DateTimeFormat().resolvedOptions().timeZone}). Runs while this
            app is open; overdue schedules run after wake or relaunch.
          </p>
          <label>
            Expected SHA-256 (optional)
            <input
              value={checksum}
              onChange={(e) => setChecksum(e.target.value)}
              pattern="[a-fA-F0-9]{64}"
              placeholder="64 hexadecimal characters"
            />
          </label>
        </details>
        <button className="primary" disabled={busy} type="submit">
          {busy ? 'Adding…' : 'Add to queue'}
        </button>
        <p role="status">{message}</p>
      </form>
    </section>
  );
}
