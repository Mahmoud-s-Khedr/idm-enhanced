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
    storage: { local: { set } },
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
