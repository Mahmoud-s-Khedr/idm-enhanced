import { testConnection } from './connection';
import { api, pending } from './browser';
import { downloadRequest, handoff } from './handoff';
import { saveRequest, finishRequest } from './handoff-state';
const button = document.querySelector<HTMLButtonElement>('#connect');
const form = document.querySelector<HTMLFormElement>('form');
const retry = document.querySelector<HTMLButtonElement>('#retry');
const status = document.querySelector<HTMLOutputElement>('output');
if (!button || !status || !form || !retry)
  throw new Error('Popup elements missing');
const send = (
  host: string,
  message: Parameters<typeof api.runtime.sendNativeMessage>[1],
) => api.runtime.sendNativeMessage(host, message);
let interacting = false;
async function show(operation: () => Promise<string>) {
  if (!status || !button || !retry) return;
  interacting = true;
  document.querySelectorAll<HTMLButtonElement>('button').forEach((b) => {
    b.disabled = true;
  });
  status.textContent = 'Connecting…';
  try {
    status.textContent = await operation();
  } catch (error: unknown) {
    status.textContent =
      error instanceof Error
        ? `${error.message} Open IDM Enhanced and check native host registration. Retry uses the same request ID.`
        : 'Desktop connection failed. Open IDM Enhanced and retry.';
  } finally {
    document.querySelectorAll<HTMLButtonElement>('button').forEach((b) => {
      b.disabled = false;
    });
  }
}
button.addEventListener('click', () => {
  void show(() => testConnection(send));
});
async function submit(reuse: boolean): Promise<string> {
  const saved = await pending();
  const url = document.querySelector<HTMLInputElement>('#url');
  const name = document.querySelector<HTMLInputElement>('#filename');
  if (reuse && !saved) throw new Error('No saved handoff to retry');
  if (reuse && saved?.accepted) return saved.status;
  const request =
    reuse && saved
      ? saved.request
      : downloadRequest(url?.value ?? '', name?.value ?? '');
  await saveRequest(request);
  try {
    const result = await handoff(send, request);
    if (await finishRequest(request, result, true))
      await api.action.setBadgeText({ text: '' });
    return result;
  } catch (error: unknown) {
    await finishRequest(
      request,
      'Not acknowledged. Open IDM Enhanced and retry this saved request.',
      false,
    );
    throw error;
  }
}
form.addEventListener('submit', (event) => {
  event.preventDefault();
  void show(() => submit(false));
});
retry.addEventListener('click', () => {
  void show(() => submit(true));
});
void pending()
  .then((saved) => {
    if (saved && status && !interacting) status.textContent = saved.status;
  })
  .catch(() => {
    if (status && !interacting)
      status.textContent =
        'Extension storage unavailable. Check browser permissions.';
  });
