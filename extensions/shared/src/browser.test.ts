import { afterEach, expect, test, vi } from 'vitest';
afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});
test('reads only well formed saved requests', async () => {
  const get = vi.fn();
  vi.stubGlobal('browser', { storage: { local: { get } } });
  const { pending } = await import('./browser');
  for (const handoff of [
    undefined,
    null,
    {},
    { status: 'x', request: {} },
    { status: 4, request: {} },
  ]) {
    get.mockResolvedValue({ handoff });
    expect(await pending()).toBeUndefined();
  }
  const handoff = {
    status: 'queued',
    request: {
      version: 1,
      type: 'download',
      id: 'id',
      url: 'https://example.invalid/a',
      filename: 'a',
    },
  };
  get.mockResolvedValue({ handoff });
  expect(await pending()).toEqual({ ...handoff, accepted: false });
});
