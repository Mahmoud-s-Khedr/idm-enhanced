// Linux acceptance smoke against a running native desktop app. Uses disposable
// browser profiles, real installed extensions, and each browser's native host.
import { spawn, execFileSync } from 'node:child_process';
import {
  mkdtempSync,
  readFileSync,
  rmSync,
  existsSync,
  writeFileSync,
  mkdirSync,
  cpSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
const ids = JSON.parse(readFileSync('extensions/identities.json', 'utf8'));
const temporary = mkdtempSync(join(tmpdir(), 'idm-browser-'));
const children = [];
const evidence = [];
async function until(operation, timeout = 15000) {
  const deadline = Date.now() + timeout;
  let last;
  while (Date.now() < deadline) {
    try {
      const value = await operation();
      if (value) return value;
    } catch (error) {
      last = error;
    }
    await delay(100);
  }
  throw new Error(`Smoke wait timed out: ${last ?? 'no result'}`);
}
async function webdriver(path, data, method = 'POST') {
  const response = await fetch(`http://127.0.0.1:4445${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    ...(data === undefined ? {} : { body: JSON.stringify(data) }),
  });
  const result = await response.json();
  if (!response.ok || result.value?.error)
    throw new Error(JSON.stringify(result));
  return result.value;
}
async function firefox() {
  const driver = spawn(
    resolve('.tools/bin/geckodriver'),
    ['--port', '4445', '--log', 'warn'],
    {
      stdio: 'ignore',
      env: { ...process.env, MOZ_REMOTE_ALLOW_SYSTEM_ACCESS: '1' },
    },
  );
  children.push(driver);
  await until(() => webdriver('/status', undefined, 'GET'));
  const { sessionId, capabilities } = await webdriver('/session', {
    capabilities: {
      alwaysMatch: {
        browserName: 'firefox',
        'moz:firefoxOptions': {
          binary: '/usr/bin/firefox',
          args: ['-headless', '--allow-system-access'],
        },
      },
    },
  });
  try {
    const xpi = join(temporary, 'firefox.xpi');
    execFileSync('zip', ['-qr', xpi, '.'], {
      cwd: resolve('extensions/firefox/dist'),
    });
    await webdriver(`/session/${sessionId}/moz/addon/install`, {
      path: xpi,
      temporary: true,
    });
    await webdriver(`/session/${sessionId}/moz/context`, { context: 'chrome' });
    const url = await until(() =>
      webdriver(`/session/${sessionId}/execute/sync`, {
        script:
          'return WebExtensionPolicy.getByID(arguments[0])?.getURL("popup.html")',
        args: [ids.firefox],
      }),
    );
    await webdriver(`/session/${sessionId}/moz/context`, {
      context: 'content',
    });
    await webdriver(`/session/${sessionId}/url`, { url });
    await webdriver(`/session/${sessionId}/execute/sync`, {
      script: 'document.querySelector("button").click(); return true;',
      args: [],
    });
    const result = await until(async () => {
      const text = await webdriver(`/session/${sessionId}/execute/sync`, {
        script: 'return document.querySelector("output").textContent',
        args: [],
      });
      if (text.startsWith('Desktop acknowledged ')) return text;
      if (text !== 'Connecting…' && text !== 'Ready to connect')
        throw new Error(text);
      return false;
    }, 8000);
    evidence.push({
      browser: 'Firefox',
      version: capabilities.browserVersion,
      result,
    });
  } finally {
    await webdriver(`/session/${sessionId}`, undefined, 'DELETE');
  }
}
async function chromium() {
  const profile = join(temporary, 'chromium');
  mkdirSync(join(profile, 'NativeMessagingHosts'), { recursive: true });
  cpSync(
    join(
      process.env.XDG_CONFIG_HOME ?? join(process.env.HOME, '.config'),
      'chromium/NativeMessagingHosts/com.idmenhanced.desktop.json',
    ),
    join(profile, 'NativeMessagingHosts/com.idmenhanced.desktop.json'),
  );

  const extension = resolve('extensions/chromium/dist');
  const browser = spawn(
    process.env.CHROMIUM_BINARY ??
      '/usr/lib64/chromium-browser/chromium-browser',
    [
      '--headless=new',
      '--no-first-run',
      '--no-default-browser-check',
      '--disable-background-networking',
      '--remote-debugging-port=0',
      `--user-data-dir=${profile}`,
      `--disable-extensions-except=${extension}`,
      `--load-extension=${extension}`,
      'about:blank',
    ],
    { stdio: 'ignore' },
  );
  children.push(browser);
  const port = await until(
    () =>
      existsSync(join(profile, 'DevToolsActivePort')) &&
      readFileSync(join(profile, 'DevToolsActivePort'), 'utf8').split('\n')[0],
  );
  const version = await (
    await fetch(`http://127.0.0.1:${port}/json/version`)
  ).json();
  const target = await (
    await fetch(
      `http://127.0.0.1:${port}/json/new?${encodeURIComponent(`chrome-extension://${ids.chromium}/popup.html`)}`,
      { method: 'PUT' },
    )
  ).json();
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    ws.addEventListener('open', resolve, { once: true });
    ws.addEventListener('error', reject, { once: true });
  });
  let serial = 0;
  const pending = new Map();
  ws.addEventListener('message', (event) => {
    const message = JSON.parse(event.data);
    const waiter = pending.get(message.id);
    if (waiter) {
      pending.delete(message.id);
      if (message.error || message.result?.exceptionDetails)
        waiter.reject(new Error(JSON.stringify(message)));
      else waiter.resolve(message.result);
    }
  });
  const evaluate = (expression) =>
    new Promise((resolve, reject) => {
      const id = ++serial;
      pending.set(id, { resolve, reject });
      ws.send(
        JSON.stringify({
          id,
          method: 'Runtime.evaluate',
          params: { expression, returnByValue: true, awaitPromise: true },
        }),
      );
    });
  try {
    await until(
      async () =>
        (await evaluate('!!document.querySelector("button")')).result.value,
    );
    await evaluate('document.querySelector("button").click()');
    const result = await until(async () => {
      const text = (
        await evaluate('document.querySelector("output").textContent')
      ).result.value;
      if (text.startsWith('Desktop acknowledged ')) return text;
      if (text !== 'Connecting…' && text !== 'Ready to connect')
        throw new Error(text);
      return false;
    }, 8000);
    evidence.push({ browser: 'Chromium', version: version.Browser, result });
  } finally {
    ws.close();
  }
}
try {
  if (process.platform !== 'linux')
    throw new Error('Live browser smoke is Linux-only in M0');
  await firefox();
  console.log(JSON.stringify(evidence.at(-1)));
  await chromium();
  const report = {
    at: new Date().toISOString(),
    platform: `${process.platform}/${process.arch}`,
    interactive: false,
    evidence,
  };
  writeFileSync(
    'build/browser-smoke.json',
    JSON.stringify(report, null, 2) + '\n',
  );
  console.log(JSON.stringify(report, null, 2));
} finally {
  for (const child of children) child.kill('SIGTERM');
  await delay(500);
  rmSync(temporary, { recursive: true, force: true });
}
