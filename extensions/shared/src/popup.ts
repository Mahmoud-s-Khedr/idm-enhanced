import { findMediaCandidates, type MediaCandidate } from './media-candidates';
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
      b.disabled = b.id === 'use-candidate' && mediaCandidates.length === 0;
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
      : downloadRequest(
          url?.value ?? '',
          name?.value ?? '',
          undefined,
          document.querySelector<HTMLInputElement>('#hls')?.checked
            ? 'hls'
            : '',
        );
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

const discover = document.querySelector<HTMLButtonElement>('#discover');
const candidates = document.querySelector<HTMLSelectElement>('#candidates');
const useCandidate =
  document.querySelector<HTMLButtonElement>('#use-candidate');
let mediaCandidates: MediaCandidate[] = [];
discover?.addEventListener('click', () => {
  void show(async () => {
    mediaCandidates = [];
    candidates?.replaceChildren();
    try {
      const [tab] = await api.tabs.query({ active: true, currentWindow: true });
      if (typeof tab?.id !== 'number') throw new Error('No active tab');
      const results = await api.scripting.executeScript({
        target: { tabId: tab.id },
        func: findMediaCandidates,
      });
      mediaCandidates = (results[0]?.result ?? []).slice(0, 32);
      for (const [index, item] of mediaCandidates.entries()) {
        const option = document.createElement('option');
        option.value = String(index);
        option.textContent = item.label;
        candidates?.append(option);
      }
      return mediaCandidates.length
        ? 'Choose a candidate, then queue it explicitly. HLS sources must match the supported VOD profile.'
        : 'No usable HTTP(S) media candidates found. Keep unsupported sources in the browser.';
    } catch {
      throw new Error(
        'Cannot inspect this tab. Use an accessible HTTP(S) page or paste a direct media URL.',
      );
    }
  });
});
useCandidate?.addEventListener('click', () => {
  const item = mediaCandidates[Number(candidates?.value)];
  if (!item) return;
  const url = document.querySelector<HTMLInputElement>('#url');
  const name = document.querySelector<HTMLInputElement>('#filename');
  const hls = document.querySelector<HTMLInputElement>('#hls');
  if (url) url.value = item.url;
  if (name) name.value = '';
  if (hls) hls.checked = item.mode === 'hls';
  if (status)
    status.textContent =
      'Media URL selected. Review the source and queue explicitly.';
});
