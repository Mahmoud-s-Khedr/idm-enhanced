import { afterEach, expect, test, vi } from 'vitest';
afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});
test('context link handoff persists before sending and presents both outcomes', async () => {
  const installed = vi.fn(),
    clicked = vi.fn(),
    create = vi.fn(),
    set = vi.fn().mockResolvedValue(undefined),
    badge = vi.fn().mockResolvedValue(undefined);
  let saved: Record<string, unknown> = {};
  const get = vi.fn().mockImplementation(() => Promise.resolve(saved));
  set.mockImplementation((value: Record<string, unknown>) => {
    saved = value;
    return Promise.resolve();
  });
  vi.stubGlobal('navigator', {
    locks: {
      request: async (_name: string, fn: () => Promise<unknown>) => fn(),
    },
  });
  const send = vi
    .fn()
    .mockImplementation((_host, m: { id: string }) =>
      Promise.resolve({ version: 1, id: m.id, type: 'accepted', jobId: 'job' }),
    );
  vi.stubGlobal('browser', undefined);
  vi.stubGlobal('chrome', {
    runtime: {
      onInstalled: { addListener: installed },
      sendNativeMessage: send,
    },
    contextMenus: { create, onClicked: { addListener: clicked } },
    storage: { local: { set, get } },
    action: { setBadgeText: badge },
  });
  await import('./background');
  installed.mock.calls[0]?.[0]();
  expect(create).toHaveBeenCalled();
  const click = clicked.mock.calls[0]?.[0] as (info: {
    menuItemId: string;
    linkUrl?: string;
  }) => void;
  click({ menuItemId: 'other' });
  expect(send).not.toHaveBeenCalled();
  click({ menuItemId: 'download', linkUrl: 'https://example.invalid/file' });
  await vi.waitFor(() => expect(badge).toHaveBeenCalledWith({ text: 'OK' }));
  expect(set.mock.invocationCallOrder[0]).toBeLessThan(
    send.mock.invocationCallOrder[0] ?? Infinity,
  );
  send.mockRejectedValue(new Error('offline'));
  click({ menuItemId: 'download', linkUrl: 'https://example.invalid/file' });
  await vi.waitFor(() => expect(badge).toHaveBeenCalledWith({ text: '!' }));
  set.mockRejectedValue(new Error('storage'));
  click({ menuItemId: 'download', linkUrl: 'https://example.invalid/file' });
  await vi.waitFor(() => expect(badge.mock.calls.length).toBe(3));
});

test('older context acknowledgement preserves the newer failed handoff for popup retry', async () => {
  let saved: Record<string, unknown> = {};
  let tail: Promise<unknown> = Promise.resolve();
  const get = vi.fn().mockImplementation(() => Promise.resolve(saved));
  const set = vi.fn().mockImplementation((value: Record<string, unknown>) => {
    saved = value;
    return Promise.resolve();
  });
  const badge = vi.fn().mockResolvedValue(undefined);
  const clicked = vi.fn();
  let completeFirst: ((reply: unknown) => void) | undefined;
  let firstID = '';
  const send = vi
    .fn()
    .mockImplementationOnce((_host, m: { id: string }) => {
      firstID = m.id;
      return new Promise((resolve) => {
        completeFirst = resolve;
      });
    })
    .mockRejectedValue(new Error('newer failure'));
  vi.stubGlobal('browser', {
    runtime: { sendNativeMessage: send, onInstalled: { addListener: vi.fn() } },
    contextMenus: { onClicked: { addListener: clicked } },
    storage: { local: { get, set } },
    action: { setBadgeText: badge },
  });
  vi.stubGlobal('navigator', {
    locks: {
      request: (_name: string, fn: () => Promise<unknown>) => {
        const run = tail.then(fn);
        tail = run.catch(() => {});
        return run;
      },
    },
  });
  await import('./background');
  const click = clicked.mock.calls[0]?.[0] as (info: {
    menuItemId: string;
    linkUrl: string;
  }) => void;
  click({ menuItemId: 'download', linkUrl: 'https://example.invalid/older' });
  await vi.waitFor(() => expect(send).toHaveBeenCalledTimes(1));
  click({ menuItemId: 'download', linkUrl: 'https://example.invalid/newer' });
  await vi.waitFor(() => expect(badge).toHaveBeenCalledWith({ text: '!' }));
  completeFirst?.({
    version: 1,
    id: firstID,
    type: 'accepted',
    jobId: 'old-job',
  });
  await vi.waitFor(() =>
    expect(get.mock.calls.length).toBeGreaterThanOrEqual(4),
  );
  const { pending } = await import('./browser');
  expect((await pending())?.request.url).toBe('https://example.invalid/newer');
  expect((await pending())?.status).toContain('Not acknowledged');
  expect(badge).toHaveBeenCalledTimes(1);
});
