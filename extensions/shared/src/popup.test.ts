import { afterEach, expect, test, vi } from 'vitest';
const send = vi.fn();
afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
  send.mockReset();
  document.body.innerHTML = '';
});
async function popup(firefox = false) {
  document.body.innerHTML =
    '<button>Test desktop connection</button><output></output>';
  const runtime = { sendNativeMessage: send };
  vi.stubGlobal('browser', firefox ? { runtime } : undefined);
  vi.stubGlobal('chrome', { runtime });
  await import('./popup');
  document.querySelector('button')?.click();
}
test('Chromium popup shows acknowledgement and restores button', async () => {
  send.mockImplementation((_host, message: { id: string }) =>
    Promise.resolve({ version: 1, id: message.id, type: 'ack' }),
  );
  await popup();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Desktop acknowledged',
    ),
  );
  await vi.waitFor(() =>
    expect(document.querySelector('button')?.disabled).toBe(false),
  );
});
test('Firefox popup presents actionable registration failure', async () => {
  send.mockRejectedValue(new Error('Host missing'));
  await popup(true);
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Host missing',
    ),
  );
});
test('handles unknown errors', async () => {
  send.mockRejectedValue('offline');
  await popup();
  await vi.waitFor(() =>
    expect(document.querySelector('output')?.textContent).toContain(
      'Desktop connection failed',
    ),
  );
});
test('missing popup DOM fails explicitly', async () => {
  vi.stubGlobal('browser', undefined);
  vi.stubGlobal('chrome', { runtime: { sendNativeMessage: send } });
  await expect(import('./popup')).rejects.toThrow('Popup elements missing');
});
