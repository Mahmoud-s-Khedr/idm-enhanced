// Real native WebView/Wails smoke. Requires an interactive display; never runs
// an installer, changes keyboard settings, or reads the user's download state.
import { createServer } from 'node:http';
import { spawn, execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createHash } from 'node:crypto';
if (!process.env.DISPLAY && !process.env.WAYLAND_DISPLAY)
  throw new Error('Native desktop smoke requires a display');
const temporary = mkdtempSync(join(tmpdir(), 'idm-desktop-'));
const data = Buffer.alloc(8 * 1024 * 1024, 'd');
const checksum = createHash('sha256').update(data).digest('hex');
const server = createServer((request, response) => {
  let start = 0,
    end = data.length - 1;
  if (request.headers.range) {
    const range = /^bytes=(\d+)-(\d*)$/.exec(request.headers.range);
    if (!range) {
      response.writeHead(416).end();
      return;
    }
    start = Number(range[1]);
    if (range[2]) end = Number(range[2]);
    response.writeHead(206, {
      'Content-Range': `bytes ${start}-${end}/${data.length}`,
      'Content-Length': end - start + 1,
      ETag: '"m4-fixture"',
    });
  } else
    response.writeHead(200, {
      'Content-Length': data.length,
      ETag: '"m4-fixture"',
    });
  const interval = setInterval(() => {
    if (start > end) {
      clearInterval(interval);
      response.end();
      return;
    }
    const next = Math.min(start + 32768, end + 1);
    response.write(data.subarray(start, next));
    start = next;
  }, 10);
  response.on('close', () => clearInterval(interval));
});
let child;
try {
  execFileSync(
    'go',
    [
      'build',
      '-tags=desktop_smoke',
      '-o',
      'build/desktop-smoke',
      './cmd/desktop',
    ],
    { stdio: 'inherit' },
  );
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const destination = join(temporary, 'output.bin');
  let logs = '';
  child = spawn(
    resolve('build/desktop-smoke'),
    ['--state-dir', join(temporary, 'state')],
    {
      env: {
        ...process.env,
        IDM_DESKTOP_SMOKE: JSON.stringify({
          url: `http://127.0.0.1:${server.address().port}/file`,
          destination,
          checksum,
        }),
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  );
  child.stdout.on('data', (b) => {
    logs += b;
    process.stdout.write(b);
  });
  child.stderr.on('data', (b) => {
    logs += b;
    process.stdout.write(b);
  });
  const timer = setTimeout(() => child.kill('SIGTERM'), 45000);
  const code = await new Promise((resolve, reject) => {
    child.on('error', reject);
    child.on('exit', resolve);
  });
  clearTimeout(timer);
  if (code !== 0 || !logs.includes('NATIVE DESKTOP SMOKE PASSED'))
    throw new Error(logs);
  if (
    createHash('sha256').update(readFileSync(destination)).digest('hex') !==
    checksum
  )
    throw new Error('Output differs or history removal deleted completed data');
  console.log(
    'PASS native WebView: settings, add, cancel focus/Escape, 200% text reflow, checksum, run, pause, resume, history, metadata-only removal, close and safe quit',
  );
} finally {
  child?.kill('SIGTERM');
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
  rmSync(temporary, { recursive: true, force: true });
}
