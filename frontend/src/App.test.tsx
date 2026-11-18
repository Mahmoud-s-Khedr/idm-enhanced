import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { vi, test, expect, beforeEach, afterEach } from 'vitest';
import { App } from './App';
import * as Service from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service';
import {
  Snapshot,
  Status,
  JobView,
} from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/models';
import { Settings } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager/models';
vi.mock(
  '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service',
  () => ({
    Info: vi.fn(),
    Hello: vi.fn(),
    Downloads: vi.fn(),
    Control: vi.fn(),
    Add: vi.fn(),
    SaveSettings: vi.fn(),
    ChooseDestination: vi.fn(),
    Quit: vi.fn(),
  }),
);
beforeEach(() => {
  vi.mocked(Service.Info).mockResolvedValue(
    new Status({ platform: 'linux', ipc: 'ready', lastPing: 'browser-123' }),
  );
  vi.mocked(Service.Downloads).mockResolvedValue(
    new Snapshot({
      settings: new Settings({
        workers: 4,
        jobs: 2,
        perHost: 4,
        retries: 3,
        rate: 0,
        notifications: true,
      }),
      jobs: [
        new JobView({
          id: 'first',
          name: 'archive.zip',
          host: 'example.com',
          state: 'queued',
          total: 100,
          bytes: 10,
        }),
        new JobView({
          id: 'second',
          name: 'done.zip',
          host: 'example.com',
          state: 'completed',
          total: 100,
          bytes: 100,
        }),
      ],
    }),
  );
  vi.mocked(Service.Control).mockResolvedValue();
});
afterEach(() => {
  vi.resetAllMocks();
});
test('filters history, searches downloads and dispatches queue commands', async () => {
  const view = render(<App />);
  await screen.findByText('archive.zip');
  fireEvent.click(screen.getByRole('button', { name: 'Run queue' }));
  await waitFor(() =>
    expect(Service.Control).toHaveBeenCalledWith(
      expect.objectContaining({ action: 'run-queue' }),
    ),
  );
  fireEvent.click(screen.getByRole('button', { name: 'History' }));
  expect(screen.queryByText('archive.zip')).not.toBeInTheDocument();
  expect(screen.getByText('done.zip')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Active & queued' }));
  expect(screen.queryByText('done.zip')).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole('searchbox'), {
    target: { value: 'missing' },
  });
  expect(screen.getByText(/No downloads here/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'All downloads' }));
  vi.mocked(Service.Control).mockRejectedValue(new Error('busy'));
  fireEvent.click(screen.getByRole('button', { name: 'Run queue' }));
  await screen.findByText(/Command failed/);
  await waitFor(() =>
    expect(vi.mocked(Service.Downloads).mock.calls.length).toBeGreaterThan(2),
  );
  view.unmount();
  const calls = vi.mocked(Service.Downloads).mock.calls.length;
  await new Promise((resolve) => setTimeout(resolve, 300));
  expect(Service.Downloads).toHaveBeenCalledTimes(calls);
});
test('opens settings, saves them and acknowledges add-download', async () => {
  render(<App />);
  await screen.findByText('archive.zip');
  fireEvent.change(screen.getByLabelText('Source URL'), {
    target: { value: 'https://example.com/new' },
  });
  fireEvent.change(screen.getByLabelText('Save as'), {
    target: { value: '/tmp/new' },
  });
  vi.mocked(Service.Add).mockResolvedValue('new');
  fireEvent.click(screen.getByRole('button', { name: 'Add to queue' }));
  await screen.findByText(/Download added/);
  fireEvent.click(screen.getByRole('button', { name: 'Settings' }));
  await screen.findByText('Download settings');
  vi.mocked(Service.SaveSettings).mockResolvedValue();
  fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));
  await screen.findByText('Settings saved.');
  fireEvent.click(screen.getByRole('button', { name: 'Settings' }));
  await screen.findByText('Add download');
});
test('presents backend failures, running/sleep states, notification fallback and safe quit', async () => {
  vi.mocked(Service.Downloads).mockRejectedValue(new Error('offline'));
  render(<App />);
  await screen.findByText(/Desktop backend disconnected/);
  vi.mocked(Service.Downloads).mockResolvedValue(
    new Snapshot({ running: true }),
  );
  vi.mocked(Service.Info).mockResolvedValue(
    new Status({ notification: 'System notifications unavailable.' }),
  );
  await screen.findByText('Downloads running');
  await screen.findByText('System notifications unavailable.');
  fireEvent.click(screen.getByRole('button', { name: 'Pause all' }));
  await screen.findByText('Command accepted.');
  vi.mocked(Service.Quit).mockRejectedValue(new Error('offline'));
  fireEvent.click(screen.getByRole('button', { name: 'Quit safely' }));
  await screen.findByText(/Unable to quit/);
  vi.mocked(Service.Hello).mockResolvedValue('Go connected');
  fireEvent.click(screen.getByText('Test backend connection'));
  await screen.findByText('Go connected');
  vi.mocked(Service.Hello).mockRejectedValue(new Error('offline'));
  fireEvent.click(screen.getByText('Test backend connection'));
  await screen.findByText('Backend connection failed.');
});
test('shows sleep and manager errors without fabricated progress', async () => {
  vi.mocked(Service.Downloads).mockResolvedValue(
    new Snapshot({ sleeping: true }),
  );
  render(<App />);
  await screen.findByText('Downloads paused for sleep.');
  vi.mocked(Service.Downloads).mockResolvedValue(
    new Snapshot({ message: 'Some downloads need attention.' }),
  );
  await screen.findByText('Some downloads need attention.');
});
