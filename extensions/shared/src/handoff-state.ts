import { api, pending } from './browser';
import type { Message } from './contract';

// Web Locks coordinate the popup and background worker across execution contexts.
// Only persistence is locked; native-message waits never hold the lock.
export async function saveRequest(request: Message): Promise<void> {
  await navigator.locks.request('idm-handoff', async () => {
    const saved = await pending();
    if (saved?.request.id === request.id) return;
    await api.storage.local.set({
      handoff: {
        request,
        status: 'Awaiting acknowledgement. Retry this request if interrupted.',
        accepted: false,
      },
    });
  });
}
export async function finishRequest(
  request: Message,
  status: string,
  accepted: boolean,
): Promise<boolean> {
  return navigator.locks.request('idm-handoff', async () => {
    const saved = await pending();
    if (
      !saved ||
      saved.request.id !== request.id ||
      (saved.accepted && !accepted)
    )
      return false;
    await api.storage.local.set({ handoff: { request, status, accepted } });
    return true;
  });
}
