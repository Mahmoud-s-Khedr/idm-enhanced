import { useState } from 'react';
import * as Service from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service';
import type { Choice } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/media/models';
import { DownloadRequest } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/models';

export function AddDownload({ added }: { added: () => void }) {
  const [url, setURL] = useState('');
  const [destination, setDestination] = useState('');
  const [checksum, setChecksum] = useState('');
  const [scheduled, setScheduled] = useState('');
  const [busy, setBusy] = useState<'' | 'analyzing' | 'adding'>('');
  const [message, setMessage] = useState('');
  const [hls, setHLS] = useState(false);
  const [dash, setDASH] = useState(false);
  const mediaMode = hls || dash;
  const format = dash ? 'DASH' : 'HLS';
  const [choices, setChoices] = useState<Choice[]>([]);
  const [choice, setChoice] = useState('');
  const analyze = async () => {
    setBusy('analyzing');
    setMessage(`Analyzing ${format} source…`);
    setChoices([]);
    setChoice('');
    try {
      setChoices(await Service.AnalyzeMedia(url));
      setMessage(
        `${format} source analyzed. Choose quality and audio, or use the highest bandwidth.`,
      );
    } catch {
      setMessage(
        `Could not analyze ${format}. Use supported unencrypted VOD: HLS MPEG-TS or static single-period DASH H.264/AAC. Live, encrypted and unsupported track layouts stay in the browser.`,
      );
    } finally {
      setBusy('');
    }
  };
  const submit = async () => {
    setBusy('adding');
    try {
      await Service.Add(
        new DownloadRequest({
          url,
          kind: dash ? 'dash' : hls ? 'hls' : '',
          mediaChoice: mediaMode ? choice : '',
          destination,
          sha256: checksum,
          scheduled: scheduled ? new Date(scheduled).toISOString() : '',
        }),
      );
      setURL('');
      setDestination('');
      setChecksum('');
      setScheduled('');
      setChoices([]);
      setChoice('');
      setMessage('Download added to the queue.');
      added();
    } catch {
      setMessage(
        mediaMode
          ? 'Could not add media. Check the HTTP playlist, selected quality and new MP4 destination.'
          : 'Could not add download. Check the HTTP URL, new destination and checksum.',
      );
    } finally {
      setBusy('');
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
        <fieldset disabled={!!busy}>
          <label>
            Source URL
            <input
              required
              type="url"
              value={url}
              placeholder="https://example.com/file.zip"
              onChange={(e) => {
                setURL(e.target.value);
                setChoices([]);
                setChoice('');
              }}
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault();
                setChoices([]);
                setChoice('');
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
          <label className="checkbox">
            <input
              type="checkbox"
              checked={hls}
              onChange={(event) => {
                setHLS(event.target.checked);
                setDASH(false);
                setChoices([]);
                setChoice('');
                setChecksum('');
              }}
            />
            HLS VOD media (MP4)
          </label>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={dash}
              onChange={(event) => {
                setDASH(event.target.checked);
                setHLS(false);
                setChoices([]);
                setChoice('');
                setChecksum('');
              }}
            />
            DASH VOD media (MP4)
          </label>
          {mediaMode && (
            <div>
              <p className="hint">
                Unencrypted HLS MPEG-TS or static DASH H.264/AAC only. Requires
                FFmpeg and ffprobe. Completed segments are retained for recovery
                until history is removed.
              </p>
              <button
                type="button"
                disabled={!!busy || !url}
                onClick={() => {
                  void analyze();
                }}
              >
                Analyze {format}
              </button>
              {choices.length > 0 && (
                <label>
                  Media quality
                  <select
                    value={choice}
                    onChange={(event) => setChoice(event.target.value)}
                  >
                    <option value="">
                      Highest bandwidth / source playlist
                    </option>
                    {choices
                      .filter((item) => item.id !== '')
                      .map((item) => (
                        <option key={item.id} value={item.id}>
                          {item.resolution || 'Unspecified resolution'} ·{' '}
                          {item.bandwidth} bit/s ·{' '}
                          {item.codecs || 'Inspect during assembly'}
                        </option>
                      ))}
                  </select>
                </label>
              )}
            </div>
          )}
          <div>
            <label htmlFor="download-destination">Save as</label>
            <span className="input-row">
              <input
                id="download-destination"
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
          </div>
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
              {Intl.DateTimeFormat().resolvedOptions().timeZone}). Runs while
              this app is open; overdue schedules run after wake or relaunch.
            </p>
            {!mediaMode && (
              <label>
                Expected SHA-256 (optional)
                <input
                  value={checksum}
                  onChange={(e) => setChecksum(e.target.value)}
                  pattern="[a-fA-F0-9]{64}"
                  placeholder="64 hexadecimal characters"
                />
              </label>
            )}
          </details>
          <button
            className="primary"
            disabled={!!busy || (mediaMode && choices.length === 0)}
            type="submit"
          >
            {busy === 'analyzing'
              ? 'Analyzing…'
              : busy === 'adding'
                ? 'Adding…'
                : 'Add to queue'}
          </button>
        </fieldset>
        <p role="status">{message}</p>
      </form>
    </section>
  );
}
