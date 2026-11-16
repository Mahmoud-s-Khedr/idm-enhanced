import { testConnection, type SendNative } from './connection';
// Firefox and current Chromium expose the Promise-based WebExtensions API.
// This declaration describes only the browser boundary, never the protocol.
declare const browser:
  { runtime: { sendNativeMessage: SendNative } } | undefined;
declare const chrome: { runtime: { sendNativeMessage: SendNative } };
const runtime =
  typeof browser === 'undefined' ? chrome.runtime : browser.runtime;
const button = document.querySelector<HTMLButtonElement>('button');
const status = document.querySelector<HTMLOutputElement>('output');
if (!button || !status) throw new Error('Popup elements missing');
button.addEventListener('click', () => {
  button.disabled = true;
  status.textContent = 'Connecting…';
  void testConnection((host, message) =>
    runtime.sendNativeMessage(host, message),
  )
    .then((reply) => {
      status.textContent = reply;
    })
    .catch((error: unknown) => {
      status.textContent =
        error instanceof Error
          ? `${error.message} Open IDM Enhanced and check native host registration.`
          : 'Desktop connection failed. Open IDM Enhanced and retry.';
    })
    .finally(() => {
      button.disabled = false;
    });
});
