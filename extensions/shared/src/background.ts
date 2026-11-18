import { api } from './browser';
import { downloadRequest, handoff } from './handoff';
api.runtime.onInstalled.addListener(() => {
  api.contextMenus.create({
    id: 'download',
    title: 'Download with IDM Enhanced',
    contexts: ['link'],
    targetUrlPatterns: ['http://*/*', 'https://*/*'],
  });
});
api.contextMenus.onClicked.addListener((info) => {
  if (info.menuItemId !== 'download' || !info.linkUrl) return;
  void (async () => {
    const request = downloadRequest(info.linkUrl ?? '', '');
    await api.storage.local.set({
      handoff: {
        request,
        status:
          'Awaiting desktop acknowledgement. Retry from the popup if interrupted.',
      },
    });
    try {
      const status = await handoff(
        (host, m) => api.runtime.sendNativeMessage(host, m),
        request,
      );
      await api.storage.local.set({ handoff: { request, status } });
      await api.action.setBadgeText({ text: 'OK' });
    } catch {
      await api.storage.local.set({
        handoff: {
          request,
          status:
            'Not acknowledged. Open IDM Enhanced, check registration, then retry this request from the popup. The browser link remains available.',
        },
      });
      await api.action.setBadgeText({ text: '!' });
    }
  })().catch(() => {
    void api.action.setBadgeText({ text: '!' });
  });
});
