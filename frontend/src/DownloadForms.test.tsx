import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { vi, test, expect, afterEach } from 'vitest';
import { AddDownload } from './AddDownload';
import { SettingsForm } from './SettingsForm';
import { Settings } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager/models';
import * as Service from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service';
vi.mock(
  '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service',
  () => ({
    Add: vi.fn(),
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
