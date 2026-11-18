import { afterEach, expect, test, vi } from 'vitest';
const send = vi.fn();
const get = vi.fn();
const set = vi.fn();
const badge = vi.fn();
const query = vi.fn();
const execute = vi.fn();
const html =
  '<button id="discover">Find</button><select id="candidates"></select><button id="use-candidate" disabled>Use</button><form><input id="url" value="https://example.invalid/file"/><input id="filename"/><input id="hls" type="checkbox"/><button type="submit">Queue</button></form><button id="retry">Retry</button><button id="connect">Connect</button><output></output>';
afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
  vi.clearAllMocks();
  document.body.innerHTML = '';
});
async function popup(firefox = false, saved: unknown = {}) {
  document.body.innerHTML = html;
  vi.stubGlobal('navigator', {
    locks: {
      request: async (_name: string, fn: () => Promise<unknown>) => fn(),
    },
  });
  get.mockResolvedValue(saved);
  set.mockResolvedValue(undefined);
  badge.mockResolvedValue(undefined);
  query.mockResolvedValue([{ id: 7 }]);
  execute.mockResolvedValue([{ result: [] }]);
  const api = {
    tabs: { query },
    scripting: { executeScript: execute },
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

test('HLS handoff retries the same persisted mode after acknowledgement loss', async () => {
  await popup();
  let saved: Record<string, unknown> = {};
  get.mockImplementation(() => Promise.resolve(saved));
  set.mockImplementation((value: Record<string, unknown>) => {
    saved = value;
    return Promise.resolve();
  });
  send
    .mockRejectedValueOnce(new Error('acknowledgement lost'))
    .mockImplementation((_host, message: { id: string }) =>
      Promise.resolve({
        version: 1,
        id: message.id,
        type: 'accepted',
        jobId: 'media-job',
      }),
    );
  const hls = document.querySelector<HTMLInputElement>('#hls');
  if (!hls) throw new Error('missing HLS choice');
  hls.checked = true;
  document.querySelector('form')?.requestSubmit();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'acknowledgement lost',
    ),
  );
  const first = send.mock.calls[0]?.[1];
  expect(first).toEqual(expect.objectContaining({ mode: 'hls' }));
  document.querySelector<HTMLButtonElement>('#retry')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Queued in desktop as media-job',
    ),
  );
  expect(send).toHaveBeenLastCalledWith(
    'com.idmenhanced.desktop',
    expect.objectContaining({ id: first.id, mode: 'hls' }),
  );
});

test('tab discovery requires an explicit candidate choice and does not queue automatically', async () => {
  await popup();
  send.mockClear();
  execute.mockResolvedValue([
    {
      result: [
        {
          url: 'https://cdn.invalid/list.m3u8?token=private',
          mode: 'hls',
          label: 'HLS candidate · cdn.invalid',
        },
        {
          url: 'https://cdn.invalid/file.mp4',
          mode: '',
          label: 'Direct media · cdn.invalid',
        },
      ],
    },
  ]);
  document.querySelector<HTMLButtonElement>('#discover')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Choose a candidate',
    ),
  );
  expect(query).toHaveBeenCalledWith({ active: true, currentWindow: true });
  expect(execute).toHaveBeenCalledWith(
    expect.objectContaining({
      target: { tabId: 7 },
      func: expect.any(Function),
    }),
  );
  document.querySelector<HTMLButtonElement>('#use-candidate')?.click();
  expect(document.querySelector<HTMLInputElement>('#url')?.value).toBe(
    'https://cdn.invalid/list.m3u8?token=private',
  );
  expect(document.querySelector<HTMLInputElement>('#hls')?.checked).toBe(true);
  expect(send).not.toHaveBeenCalled();
  const select = document.querySelector<HTMLSelectElement>('#candidates');
  if (!select) throw new Error('missing choices');
  select.value = '1';
  document.querySelector<HTMLButtonElement>('#use-candidate')?.click();
  expect(document.querySelector<HTMLInputElement>('#hls')?.checked).toBe(false);
  execute.mockResolvedValue([{ result: [] }]);
  document.querySelector<HTMLButtonElement>('#discover')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'No usable HTTP(S) media',
    ),
  );
  expect(
    document.querySelector<HTMLButtonElement>('#use-candidate')?.disabled,
  ).toBe(true);
  query.mockResolvedValue([]);
  document.querySelector<HTMLButtonElement>('#discover')?.click();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Cannot inspect this tab',
    ),
  );
});
