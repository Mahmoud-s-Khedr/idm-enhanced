// Shared immutable local HLS/DASH fixture bytes for native and browser acceptance.
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
const paths = [
  'master.m3u8',
  ...['low', 'high'].flatMap((quality) =>
    ['index.m3u8', 'segment-00.ts', 'segment-01.ts'].map(
      (name) => `${quality}/${name}`,
    ),
  ),
];
const bodies = new Map(
  paths.map((path) => [
    `/hls/${path}`,
    readFileSync(join('testdata/hls', path)),
  ]),
);
for (const path of readdirSync('testdata/dash').filter((name) =>
  /\.(mpd|mp4|m4s)$/.test(name),
)) {
  bodies.set(`/dash/${path}`, readFileSync(join('testdata/dash', path)));
}
export function mediaFixture(path) {
  return bodies.get(path?.split('?')[0]);
}
