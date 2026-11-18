import { afterEach, expect, test, vi } from 'vitest';
import { findMediaCandidates } from './media-candidates';
afterEach(() => {
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});
test('discovers explicit media elements and recent manifest URLs without credentials or query labels', () => {
  document.body.innerHTML =
    '<video src="https://cdn.invalid/movie.mp4#play"></video><audio><source src="https://cdn.invalid/audio"></audio><source src="blob:https://cdn.invalid/id"><source src="file:///private"><source src="https://name:pass@cdn.invalid/private"><source src="http://["><video></video>';
  vi.stubGlobal('performance', {
    getEntriesByType: () => [
      { name: 'https://cdn.invalid/movie.mp4' },
      { name: 'https://cdn.invalid/list.m3u8?token=private#part' },
      { name: 'https://cdn.invalid/style.css' },
      { name: 'https://cdn.invalid/tone.ogg' },
    ],
  });
  const result = findMediaCandidates();
  expect(result.map((item) => item.url)).toEqual(
    expect.arrayContaining([
      'https://cdn.invalid/movie.mp4',
      'https://cdn.invalid/audio',
      'https://cdn.invalid/list.m3u8?token=private',
      'https://cdn.invalid/tone.ogg',
    ]),
  );
  expect(result).toHaveLength(4);
  expect(result.find((item) => item.mode === 'hls')?.label).toContain(
    'HLS candidate',
  );
  expect(result.map((item) => item.label).join(' ')).not.toContain('private');
});
test('bounds DOM inspection, resource history, candidate count and URL sizes', () => {
  document.body.innerHTML =
    '<source src="' +
    'x'.repeat(16385) +
    '">' +
    Array.from(
      { length: 80 },
      (_, i) => `<video src="https://cdn.invalid/${i}.mp4"></video>`,
    ).join('');
  vi.stubGlobal('performance', {
    getEntriesByType: () =>
      Array.from({ length: 300 }, (_, i) => ({
        name: `https://cdn.invalid/${i}.m3u8`,
      })),
  });
  expect(findMediaCandidates()).toHaveLength(32);
  document.body.replaceChildren();
  const result = findMediaCandidates();
  expect(result).toHaveLength(32);
  expect(result[0]?.url).toContain('/299.m3u8');
  expect(result.some((item) => item.url.endsWith('/0.m3u8'))).toBe(false);
  vi.stubGlobal('performance', {
    getEntriesByType: () => [
      { name: 'https://cdn.invalid/' + 'x'.repeat(16400) + '.m3u8' },
    ],
  });
  expect(findMediaCandidates()).toEqual([]);
});
