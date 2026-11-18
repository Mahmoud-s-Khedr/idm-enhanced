import { expect, test, vi } from 'vitest';
import { downloadRequest, handoff } from './handoff';
import { decode } from './connection';
test('builds safe direct links without leaking credentials or accepting browser URLs', () => {
  expect(
    downloadRequest('https://example.invalid/path/file.bin#part', '', 'id'),
  ).toEqual({
    version: 1,
    id: 'id',
    type: 'download',
    url: 'https://example.invalid/path/file.bin',
    filename: 'file.bin',
  });
  for (const source of [
    'blob:https://example.invalid/id',
    'file:///tmp/a',
    'https://user:pass@example.invalid/a',
  ])
    expect(() => downloadRequest(source, '')).toThrow();
  for (const path of ['/%ZZ', '/CON.txt', '/', '/' + 'a'.repeat(181)])
    expect(downloadRequest('https://example.invalid' + path, '').filename).toBe(
      'download',
    );
  expect(downloadRequest('https://example.invalid', '../bad:*').filename).toBe(
    '.._bad__',
  );
  expect(() =>
    downloadRequest('https://example.invalid/' + 'a'.repeat(17000), 'a'),
  ).toThrow('too long');
});
test('requires matching durable job acknowledgement', async () => {
  const request = downloadRequest('https://example.invalid/a', 'a', 'id');
  expect(
    await handoff(
      vi.fn().mockResolvedValue({
        version: 1,
        id: 'id',
        type: 'accepted',
        jobId: 'job',
      }),
      request,
    ),
  ).toContain('job');
  await expect(
    handoff(
      vi.fn().mockResolvedValue({ version: 1, id: 'id', type: 'ack' }),
      request,
    ),
  ).rejects.toThrow();
  for (const value of [
    { version: 1, id: 'x', type: 'accepted' },
    { version: 1, id: 'x', type: 'accepted', jobId: 'job', error: 'bad' },
    { version: 1, id: 'x', type: 'ack', jobId: 'job' },
  ])
    expect(() => decode(value)).toThrow();
});
