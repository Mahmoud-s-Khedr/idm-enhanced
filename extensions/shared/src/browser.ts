import type { MediaCandidate } from './media-candidates';
import type { Message } from './contract';
import type { SendNative } from './connection';

interface BrowserAPI {
  tabs: {
    query(options: {
      active: boolean;
      currentWindow: boolean;
    }): Promise<Array<{ id?: number }>>;
  };
  scripting: {
    executeScript(options: {
      target: { tabId: number };
      func: () => MediaCandidate[];
    }): Promise<Array<{ result?: MediaCandidate[] }>>;
  };
  runtime: {
    sendNativeMessage: SendNative;
    onInstalled: { addListener(fn: () => void): void };
  };
  contextMenus: {
    create(
      options: {
        id: string;
        title: string;
        contexts: string[];
        targetUrlPatterns: string[];
      },
      callback?: () => void,
    ): void;
    onClicked: {
      addListener(
        fn: (info: { menuItemId: string | number; linkUrl?: string }) => void,
      ): void;
    };
  };
  storage: {
    local: {
      get(key: string): Promise<Record<string, unknown>>;
      set(data: Record<string, unknown>): Promise<void>;
    };
  };
  action: { setBadgeText(options: { text: string }): Promise<void> };
}
declare const browser: BrowserAPI | undefined;
declare const chrome: BrowserAPI;
export const api = typeof browser === 'undefined' ? chrome : browser;
export interface Pending {
  request: Message;
  status: string;
  accepted: boolean;
}
export async function pending(): Promise<Pending | undefined> {
  const value = (await api.storage.local.get('handoff')).handoff;
  if (
    typeof value !== 'object' ||
    value === null ||
    !('request' in value) ||
    !('status' in value) ||
    typeof value.status !== 'string'
  )
    return undefined;
  const r = value.request;
  if (
    typeof r !== 'object' ||
    r === null ||
    !('version' in r) ||
    r.version !== 1 ||
    !('type' in r) ||
    r.type !== 'download' ||
    !('id' in r) ||
    typeof r.id !== 'string' ||
    !('url' in r) ||
    typeof r.url !== 'string' ||
    !('filename' in r) ||
    typeof r.filename !== 'string'
  )
    return undefined;
  if ('mode' in r && r.mode !== '' && r.mode !== 'hls') return undefined;
  return {
    request: {
      version: 1,
      type: 'download',
      id: r.id,
      url: r.url,
      filename: r.filename,
      ...('mode' in r && r.mode === 'hls' ? { mode: 'hls' } : {}),
    },
    status: value.status,
    accepted: 'accepted' in value && value.accepted === true,
  };
}
