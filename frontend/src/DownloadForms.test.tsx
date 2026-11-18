import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { vi, test, expect, afterEach } from 'vitest';
import { CancellablePromise } from '@wailsio/runtime';
import { Choice } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/media/models';
import { AddDownload } from './AddDownload';
import { SettingsForm } from './SettingsForm';
import { Settings } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager/models';
import * as Service from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service';
vi.mock(
  '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service',
  () => ({
    Add: vi.fn(),
    AnalyzeMedia: vi.fn(),
    ChooseDestination: vi.fn(),
    SaveSettings: vi.fn(),
    Control: vi.fn(),
  }),
);
afterEach(() => {
  vi.resetAllMocks();
});
test('adds a scheduled verified download and handles chooser cancellation/failure', async () => {
  const added = vi.fn();
  render(<AddDownload added={added} />);
  vi.mocked(Service.ChooseDestination).mockResolvedValue('/tmp/file');
  fireEvent.click(screen.getByText('Browse…'));
  await waitFor(() =>
    expect(screen.getByLabelText('Save as')).toHaveValue('/tmp/file'),
  );
  vi.mocked(Service.ChooseDestination).mockResolvedValue('');
  fireEvent.click(screen.getByText('Browse…'));
  await waitFor(() =>
    expect(Service.ChooseDestination).toHaveBeenCalledTimes(2),
  );
  expect(screen.getByLabelText('Save as')).toHaveValue('/tmp/file');
  vi.mocked(Service.ChooseDestination).mockRejectedValue(
    new Error('unavailable'),
  );
  fireEvent.click(screen.getByText('Browse…'));
  await screen.findByText(/Destination chooser unavailable/);
  fireEvent.dragOver(screen.getByLabelText('Source URL'));
  fireEvent.drop(screen.getByLabelText('Source URL'), {
    dataTransfer: {
      types: ['text/uri-list'],
      getData: () => '# comment\nhttps://example.com/file',
    },
  });
  fireEvent.change(screen.getByLabelText('Start at (local time)'), {
    target: { value: '2030-01-01T12:00' },
  });
  fireEvent.change(screen.getByLabelText('Expected SHA-256 (optional)'), {
    target: { value: 'a'.repeat(64) },
  });
  vi.mocked(Service.Add).mockResolvedValue('id');
  fireEvent.click(screen.getByText('Add to queue'));
  await screen.findByText(/Download added/);
  expect(Service.Add).toHaveBeenCalledWith(
    expect.objectContaining({
      url: 'https://example.com/file',
      destination: '/tmp/file',
      sha256: 'a'.repeat(64),
      scheduled: new Date('2030-01-01T12:00').toISOString(),
    }),
  );
  expect(added).toHaveBeenCalled();
  fireEvent.drop(screen.getByLabelText('Source URL'), {
    dataTransfer: {
      types: ['text/uri-list'],
      getData: (type: string) =>
        type === 'text/plain' ? 'https://example.com/plain' : '',
    },
  });
  fireEvent.change(screen.getByLabelText('Save as'), {
    target: { value: '/tmp/plain' },
  });
  vi.mocked(Service.Add).mockRejectedValue(new Error('collision'));
  fireEvent.click(screen.getByText('Add to queue'));
  await screen.findByText(/Could not add download/);
});
test('edits bounded settings and keeps an actionable error on save failure', async () => {
  const saved = vi.fn();
  render(
    <SettingsForm
      settings={
        new Settings({
          workers: 4,
          jobs: 2,
          perHost: 4,
          retries: 3,
          rate: 0,
          notifications: true,
        })
      }
      saved={saved}
    />,
  );
  fireEvent.change(screen.getByLabelText('Range workers'), {
    target: { value: '8' },
  });
  fireEvent.click(screen.getByLabelText('System completion notifications'));
  vi.mocked(Service.SaveSettings).mockRejectedValue(new Error('busy'));
  fireEvent.click(screen.getByText('Save settings'));
  await screen.findByText(/Could not save settings/);
  vi.mocked(Service.SaveSettings).mockResolvedValue();
  fireEvent.click(screen.getByText('Save settings'));
  await screen.findByText('Settings saved.');
  expect(Service.SaveSettings).toHaveBeenLastCalledWith(
    expect.objectContaining({ workers: 8, notifications: false }),
  );
  expect(saved).toHaveBeenCalled();
  vi.mocked(Service.Control).mockResolvedValue();
  fireEvent.click(screen.getByText('Authorize system notifications'));
  await screen.findByText('Notification permission granted.');
  vi.mocked(Service.Control).mockRejectedValue(new Error('denied'));
  fireEvent.click(screen.getByText('Authorize system notifications'));
  await screen.findByText(
    'System notifications unavailable. Completion remains in History.',
  );
});

test.each(['HLS', 'DASH'])(
  'analyzes %s, locks edits and queues selected tracks',
  async (format) => {
    const added = vi.fn();
    render(<AddDownload added={added} />);
    fireEvent.click(screen.getByLabelText(`${format} VOD media (MP4)`));
    expect(
      screen.queryByLabelText('Expected SHA-256 (optional)'),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: `Analyze ${format}` }),
    ).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Add to queue' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Source URL'), {
      target: { value: 'https://example.com/master.m3u8' },
    });
    fireEvent.change(screen.getByLabelText('Save as'), {
      target: { value: '/tmp/media.mp4' },
    });
    let complete: ((choices: Choice[]) => void) | undefined;
    vi.mocked(Service.AnalyzeMedia).mockImplementation(
      () =>
        new CancellablePromise<Choice[]>((resolve) => {
          complete = resolve;
        }),
    );
    fireEvent.click(screen.getByRole('button', { name: `Analyze ${format}` }));
    expect(screen.getByLabelText('Source URL')).toBeDisabled();
    expect(screen.getByLabelText(`${format} VOD media (MP4)`)).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Analyzing…' })).toBeDisabled();
    const selectedChoice = new Choice({
      id: 'a'.repeat(64),
      bandwidth: 1000,
      resolution: '128x72',
      codecs: 'avc1.42c01e,mp4a.40.2',
    });
    const choices = [
      selectedChoice,
      new Choice({ id: 'b'.repeat(64), bandwidth: 2000 }),
    ];
    await act(async () => {
      complete?.(choices);
    });
    await screen.findByLabelText('Media quality');
    fireEvent.change(screen.getByLabelText('Media quality'), {
      target: { value: selectedChoice.id },
    });
    expect(screen.getByLabelText('Source URL')).toBeEnabled();
    vi.mocked(Service.Add)
      .mockRejectedValueOnce(new Error('destination collision'))
      .mockResolvedValue('media-id');
    fireEvent.click(screen.getByRole('button', { name: 'Add to queue' }));
    await screen.findByText(/Could not add media/);
    fireEvent.click(screen.getByRole('button', { name: 'Add to queue' }));
    await screen.findByText('Download added to the queue.');
    expect(Service.Add).toHaveBeenLastCalledWith(
      expect.objectContaining({
        kind: format.toLowerCase(),
        mediaChoice: selectedChoice.id,
        url: 'https://example.com/master.m3u8',
        destination: '/tmp/media.mp4',
        sha256: '',
      }),
    );
    expect(added).toHaveBeenCalledOnce();
    expect(screen.queryByLabelText('Media quality')).not.toBeInTheDocument();
    fireEvent.click(screen.getByLabelText(`${format} VOD media (MP4)`));
    expect(
      screen.getByLabelText('Expected SHA-256 (optional)'),
    ).toBeInTheDocument();
  },
);

test('HLS errors retain browser fallback and changing source invalidates previous choices', async () => {
  render(<AddDownload added={vi.fn()} />);
  fireEvent.click(screen.getByLabelText('HLS VOD media (MP4)'));
  fireEvent.change(screen.getByLabelText('Source URL'), {
    target: { value: 'https://example.com/unsupported' },
  });
  vi.mocked(Service.AnalyzeMedia)
    .mockRejectedValueOnce(new Error('private diagnostic'))
    .mockResolvedValue([new Choice({ id: '' })]);
  fireEvent.click(screen.getByRole('button', { name: 'Analyze HLS' }));
  await screen.findByText(/Could not analyze HLS/);
  expect(screen.queryByText('private diagnostic')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Add to queue' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Analyze HLS' }));
  await screen.findByLabelText('Media quality');
  fireEvent.change(screen.getByLabelText('Source URL'), {
    target: { value: 'https://example.com/new.m3u8' },
  });
  expect(screen.queryByLabelText('Media quality')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Add to queue' })).toBeDisabled();
});
