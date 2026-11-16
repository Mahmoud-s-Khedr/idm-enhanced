import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { vi, test, expect, afterEach } from 'vitest';
import { App } from './App';
import * as Service from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service';
vi.mock(
  '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/service',
  () => ({ Info: vi.fn(), Hello: vi.fn() }),
);
afterEach(() => {
  vi.resetAllMocks();
  vi.useRealTimers();
});
test('displays backend response and acknowledged browser request', async () => {
  vi.mocked(Service.Info).mockResolvedValue({
    build: 'M0',
    platform: 'linux/amd64',
    lastPing: 'browser-123',
    ipc: 'ready',
  });
  vi.mocked(Service.Hello).mockResolvedValue('Hello from Go');
  render(<App />);
  expect(await screen.findByText('Connected')).toBeInTheDocument();
  expect(screen.getByText('browser-123')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button'));
  expect(await screen.findByText('Hello from Go')).toBeInTheDocument();
});
test('shows disconnected state and actionable backend failure', async () => {
  vi.mocked(Service.Info).mockRejectedValue(new Error('offline'));
  vi.mocked(Service.Hello).mockRejectedValue(new Error('offline'));
  render(<App />);
  expect(
    await screen.findByText(/Desktop backend disconnected/),
  ).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button'));
  expect(await screen.findByText(/Backend call failed/)).toBeInTheDocument();
});
test('polls connection and stops polling on unmount', async () => {
  vi.mocked(Service.Info).mockResolvedValue({
    build: 'M0',
    platform: 'linux',
    lastPing: '',
    ipc: 'ready',
  });
  const view = render(<App />);
  await screen.findByText('Connected');
  await waitFor(
    () => {
      expect(Service.Info).toHaveBeenCalledTimes(2);
    },
    { timeout: 2000 },
  );
  view.unmount();
});
