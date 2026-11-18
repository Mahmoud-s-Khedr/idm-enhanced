import { api } from './browser';
import { downloadRequest, handoff } from './handoff';
import { saveRequest, finishRequest } from './handoff-state';
api.runtime.onInstalled.addListener(() => {
  api.contextMenus.create({
    id: 'download',
    title: 'Download with IDM Enhanced',
    contexts: ['link'],
    targetUrlPatterns: ['http://*/*', 'https://*/*'],
  });
  api.contextMenus.create({
    id: 'download-hls',
    title: 'Queue HLS VOD with IDM Enhanced (highest quality)',
    contexts: ['link'],
    targetUrlPatterns: ['http://*/*', 'https://*/*'],
  });
});
api.contextMenus.onClicked.addListener((info) => {
  const source = info.linkUrl;
  if (
    !['download', 'download-hls'].includes(String(info.menuItemId)) ||
    !source
  )
    return;
  void (async () => {
    const request = downloadRequest(
      source,
      '',
      undefined,
      info.menuItemId === 'download-hls' ? 'hls' : '',
    );
    await saveRequest(request);
    try {
      const status = await handoff(
        (host, m) => api.runtime.sendNativeMessage(host, m),
        request,
      );
      if (await finishRequest(request, status, true))
        await api.action.setBadgeText({ text: 'OK' });
    } catch {
      if (
        await finishRequest(
          request,
          'Not acknowledged. Open IDM Enhanced, check registration, then retry this request from the popup. The browser link remains available.',
          false,
        )
      )
        await api.action.setBadgeText({ text: '!' });
    }
  })().catch(() => {
    void api.action.setBadgeText({ text: '!' });
  });
});
