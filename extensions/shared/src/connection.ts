import { VERSION, MAX_MESSAGE, TIMEOUT_MS, type Message } from './contract';
export const HOST = 'com.idmenhanced.desktop';
export type SendNative = (host: string, message: Message) => Promise<unknown>;
export function decode(value: unknown): Message {
  if (new TextEncoder().encode(JSON.stringify(value)).length > MAX_MESSAGE)
    throw new Error('Message exceeds 64 KiB');
  if (typeof value !== 'object' || value === null)
    throw new Error('Malformed desktop reply');
  if (
    !('version' in value) ||
    value.version !== VERSION ||
    !('id' in value) ||
    typeof value.id !== 'string' ||
    !/^[A-Za-z0-9_-]{1,64}$/.test(value.id) ||
    !('type' in value) ||
    typeof value.type !== 'string' ||
    !['ping', 'ack', 'error', 'accepted'].includes(value.type)
  )
    throw new Error('Invalid desktop reply');
  if (
    Object.keys(value).some(
      (key) => !['version', 'id', 'type', 'error', 'jobId'].includes(key),
    )
  )
    throw new Error('Unknown message field');
  if (value.type === 'accepted') {
    if (
      !('jobId' in value) ||
      typeof value.jobId !== 'string' ||
      !/^[A-Za-z0-9_-]{1,64}$/.test(value.jobId) ||
      'error' in value
    )
      throw new Error('Invalid accepted reply');
    return {
      version: VERSION,
      id: value.id,
      type: 'accepted',
      jobId: value.jobId,
    };
  }
  if ('jobId' in value) throw new Error('Unexpected job ID');
  if (value.type === 'error') {
    if (
      !('error' in value) ||
      typeof value.error !== 'string' ||
      value.error.length === 0 ||
      new TextEncoder().encode(value.error).length > 512
    )
      throw new Error('Invalid error reply');
    return {
      version: VERSION,
      id: value.id,
      type: 'error',
      error: value.error,
    };
  }
  if ('error' in value && value.error !== '')
    throw new Error('Unexpected error field');
  return { version: VERSION, id: value.id, type: value.type };
}
export async function exchange(
  send: SendNative,
  request: Message,
  timeout = TIMEOUT_MS,
): Promise<Message> {
  const id = request.id;
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const response = decode(
      await Promise.race([
        send(HOST, request),
        new Promise<never>((_, reject) => {
          timer = setTimeout(
            () =>
              reject(
                new Error(
                  'Desktop connection timed out. Open IDM Enhanced and retry.',
                ),
              ),
            timeout,
          );
        }),
      ]),
    );
    if (response.id !== id) throw new Error('Acknowledgement ID mismatch');
    if (response.type === 'error') throw new Error(response.error);
    if (response.type !== (request.type === 'download' ? 'accepted' : 'ack'))
      throw new Error('Expected desktop acknowledgement');
    return response;
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
}

export async function testConnection(
  send: SendNative,
  id: string = crypto.randomUUID(),
  timeout = TIMEOUT_MS,
): Promise<string> {
  const reply = await exchange(
    send,
    decode({ version: VERSION, id, type: 'ping' }),
    timeout,
  );
  return `Desktop acknowledged ${reply.id}`;
}
