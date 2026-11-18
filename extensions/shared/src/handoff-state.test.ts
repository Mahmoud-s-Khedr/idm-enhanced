import { afterEach, expect, test, vi } from 'vitest';
import type { Message } from './contract';
afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});
test('a late context reply cannot replace a newer popup request or erase acceptance', async () => {
  let stored: Record<string, unknown> = {};
  let tail: Promise<unknown> = Promise.resolve();
  const get = vi.fn().mockImplementation(() => Promise.resolve(stored));
  const set = vi.fn().mockImplementation((value: Record<string, unknown>) => {
    stored = value;
    return Promise.resolve();
  });
  vi.stubGlobal('browser', { storage: { local: { get, set } } });
  vi.stubGlobal('navigator', {
    locks: {
      request: (_name: string, fn: () => Promise<unknown>) => {
        const run = tail.then(fn);
        tail = run.catch(() => {});
        return run;
      },
    },
  });
  const { saveRequest, finishRequest } = await import('./handoff-state');
  const { pending } = await import('./browser');
  const a: Message = {
    version: 1,
    id: 'older',
    type: 'download',
    url: 'https://example.invalid/a',
    filename: 'a',
  };
  const b = { ...a, id: 'newer', filename: 'b' };
  expect(await finishRequest(a, 'orphan', true)).toBe(false);
  await saveRequest(a);
  await saveRequest(b);
  expect(await finishRequest(b, 'Latest request failed', false)).toBe(true);
  expect(await finishRequest(a, 'Old request accepted', true)).toBe(false);
  expect((await pending())?.request.id).toBe('newer');
  expect((await pending())?.status).toBe('Latest request failed');
  await saveRequest(b);
  expect((await pending())?.status).toBe('Latest request failed');
  expect(await finishRequest(b, 'Accepted in desktop', true)).toBe(true);
  expect(await finishRequest(b, 'Late timeout', false)).toBe(false);
  expect((await pending())?.status).toBe('Accepted in desktop');
  await Promise.all([
    saveRequest(a),
    saveRequest(b),
    finishRequest(a, 'Older response', false),
  ]);
  expect((await pending())?.request.id).toBe('newer');
  set.mockRejectedValueOnce(new Error('storage unavailable'));
  await expect(finishRequest(b, 'update', true)).rejects.toThrow(
    'storage unavailable',
  );
});
