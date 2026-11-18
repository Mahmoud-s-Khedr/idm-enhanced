import { VERSION, type Message } from './contract';
import { exchange, type SendNative } from './connection';

export function downloadRequest(
  source: string,
  filename: string,
  id: string = crypto.randomUUID(),
  mode: '' | 'hls' = '',
): Message {
  const url = new URL(source);
  if (
    !['http:', 'https:'].includes(url.protocol) ||
    url.username ||
    url.password
  )
    throw new Error(
      'Only direct HTTP(S) links are supported. Keep authenticated or browser-generated downloads in the browser.',
    );
  url.hash = '';
  let suggested = filename;
  if (!suggested) {
    try {
      suggested = decodeURIComponent(url.pathname.split('/').at(-1) ?? '');
    } catch {
      suggested = '';
    }
  }
  suggested = suggested
    .replace(/[\x00-\x1f\x7f/\\:<>"|?*]/g, '_')
    .replace(/[ .]+$/g, '');
  if (
    !suggested ||
    new TextEncoder().encode(suggested).length > 180 ||
    /^(con|prn|aux|nul|com\d|lpt\d)(\.|$)/i.test(suggested)
  )
    suggested = 'download';
  if (url.href.length > 16384) throw new Error('Download URL is too long');
  return {
    version: VERSION,
    id,
    type: 'download',
    url: url.href,
    filename: suggested,
    ...(mode ? { mode } : {}),
  };
}
export async function handoff(
  send: SendNative,
  request: Message,
): Promise<string> {
  const reply = await exchange(send, request);
  return `Queued in desktop as ${reply.jobId}. Open IDM Enhanced and run the queue.`;
}
