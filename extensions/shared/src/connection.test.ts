import { test, expect, vi } from 'vitest';
import fixtures from '../../../testdata/protocol/messages.json';
import { decode, HOST, testConnection } from './connection';
for (const fixture of fixtures)
  test(fixture.name, () => {
    if (fixture.valid) expect(() => decode(fixture.message)).not.toThrow();
    else expect(() => decode(fixture.message)).toThrow();
  });
test('requires bounded well-formed JSON messages', () => {
  for (const value of [
    null,
    false,
    'x'.repeat(65537),
    {},
    { version: 1, id: 'x', type: 'error', error: 'x'.repeat(513) },
  ])
    expect(() => decode(value)).toThrow();
});
test('matches acknowledgement and host', async () => {
  const send = vi
    .fn()
    .mockResolvedValue({ version: 1, id: 'request', type: 'ack' });
  expect(await testConnection(send, 'request')).toContain('request');
  expect(send).toHaveBeenCalledWith(HOST, {
    version: 1,
    id: 'request',
    type: 'ping',
  });
});
test('rejects wrong ID, reply type, error, transport failure and timeout', async () => {
  for (const reply of [
    { version: 1, id: 'other', type: 'ack' },
    { version: 1, id: 'request', type: 'ping' },
    { version: 1, id: 'request', type: 'error', error: 'Open desktop' },
  ])
    await expect(
      testConnection(vi.fn().mockResolvedValue(reply), 'request'),
    ).rejects.toThrow();
  await expect(
    testConnection(
      vi.fn().mockRejectedValue(new Error('not registered')),
      'request',
    ),
  ).rejects.toThrow('not registered');
  await expect(
    testConnection(() => new Promise(() => {}), 'request', 5),
  ).rejects.toThrow('timed out');
});
