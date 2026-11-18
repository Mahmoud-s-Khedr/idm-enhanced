// Shared immutable local HLS fixture bytes for native and browser acceptance.
import { readFileSync } from 'node:fs';
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
export function hlsFixture(path) {
  return bodies.get(path?.split('?')[0]);
}
