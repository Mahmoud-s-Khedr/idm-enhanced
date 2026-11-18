import { afterEach, expect, test, vi } from 'vitest';
const send = vi.fn();
const get = vi.fn();
const set = vi.fn();
const badge = vi.fn();
const html =
  '<form><input id="url" value="https://example.invalid/file"/><input id="filename"/><button type="submit">Queue</button></form><button id="retry">Retry</button><button id="connect">Connect</button><output></output>';
afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
  vi.clearAllMocks();
  document.body.innerHTML = '';
});
async function popup(firefox = false, saved: unknown = {}) {
  document.body.innerHTML = html;
  get.mockResolvedValue(saved);
  set.mockResolvedValue(undefined);
  badge.mockResolvedValue(undefined);
  const api = {
    runtime: { sendNativeMessage: send },
    storage: { local: { get, set } },
    action: { setBadgeText: badge },
  };
  vi.stubGlobal('browser', firefox ? api : undefined);
  vi.stubGlobal('chrome', api);
  await import('./popup');
}
test('connection acknowledgement and actionable failures', async () => {
  send.mockImplementation((_host, m: { id: string }) =>
    Promise.resolve({ version: 1, id: m.id, type: 'ack' }),
  );
  await popup();
  document.querySelector<HTMLButtonElement>('#connect')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Desktop acknowledged',
    ),
  );
  await vi.waitFor(() =>
    expect(
      document.querySelector<HTMLButtonElement>('#connect')?.disabled,
    ).toBe(false),
  );
  send.mockRejectedValue(new Error('Host missing'));
  document.querySelector<HTMLButtonElement>('#connect')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Host missing',
    ),
  );
  send.mockRejectedValue('offline');
  document.querySelector<HTMLButtonElement>('#connect')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Desktop connection failed',
    ),
  );
});
test('queues then retries the exact durable request after a lost reply', async () => {
  await popup(true);
  send.mockRejectedValue(new Error('lost reply'));
  document
    .querySelector('form')
    ?.dispatchEvent(new Event('submit', { cancelable: true }));
  await vi.waitFor(() => expect(set).toHaveBeenCalled());
  await vi.waitFor(() =>
    expect(document.querySelector<HTMLButtonElement>('#retry')?.disabled).toBe(
      false,
    ),
  );
  const record = set.mock.calls[0]?.[0] as {
    handoff: { request: { id: string } };
  };
  get.mockResolvedValue(record);
  send.mockImplementation((_host, m: { id: string }) =>
    Promise.resolve({
      version: 1,
      id: m.id,
      type: 'accepted',
      jobId: 'durable',
    }),
  );
  document.querySelector<HTMLButtonElement>('#retry')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Queued in desktop',
    ),
  );
  expect(send.mock.calls[1]?.[1]).toEqual(record.handoff.request);
});
test('restores saved feedback and reports storage failures', async () => {
  await popup();
  get.mockRejectedValue(new Error('storage'));
  document.querySelector<HTMLButtonElement>('#retry')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain('storage'),
  );
});
test('missing DOM fails explicitly', async () => {
  vi.stubGlobal('browser', undefined);
  vi.stubGlobal('chrome', {});
  await expect(import('./popup')).rejects.toThrow('Popup elements missing');
});

test('shows saved outcome after popup reopen', async () => {
  await popup(false, {
    handoff: {
      status: 'Already queued',
      request: {
        version: 1,
        type: 'download',
        id: 'id',
        url: 'https://example.invalid/a',
        filename: 'a',
      },
    },
  });
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toBe(
      'Already queued',
    ),
  );
});

test('empty retry does not send a new download', async () => {
  await popup();
  document.querySelector<HTMLButtonElement>('#retry')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'No saved handoff',
    ),
  );
  expect(send).not.toHaveBeenCalled();
});
