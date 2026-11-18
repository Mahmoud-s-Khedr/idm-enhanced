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

test('saved HLS retries retain mode and unsupported modes are rejected', async () => {
  const get = vi.fn();
  vi.stubGlobal('browser', { storage: { local: { get } } });
  const { pending } = await import('./browser');
  const request = {
    version: 1,
    type: 'download',
    id: 'media',
    url: 'https://example.invalid/list',
    filename: 'list',
    mode: 'hls',
  };
  get.mockResolvedValue({
    handoff: { request, status: 'retry', accepted: false },
  });
  expect((await pending())?.request).toEqual(request);
  get.mockResolvedValue({
    handoff: { request: { ...request, mode: 'unknown' }, status: 'retry' },
  });
  expect(await pending()).toBeUndefined();
  get.mockResolvedValue({
    handoff: { request: { ...request, mode: '' }, status: 'retry' },
  });
  expect((await pending())?.request.mode).toBeUndefined();
});
