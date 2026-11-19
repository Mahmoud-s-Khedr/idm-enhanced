export interface MediaCandidate {
  url: string;
  mode: '' | 'hls' | 'dash';
  label: string;
}

// Serialized by scripting.executeScript: keep every runtime dependency inside
// this function. Read only the current main frame after an explicit user action.
export function findMediaCandidates(): MediaCandidate[] {
  const result: MediaCandidate[] = [];
  const seen = new Set<string>();
  const add = (value: string, observed: boolean) => {
    if (!value || value.length > 16384 || result.length >= 32) return;
    try {
      const url = new URL(value, document.baseURI);
      if (
        !['http:', 'https:'].includes(url.protocol) ||
        url.username ||
        url.password ||
        url.href.length > 16384
      )
        return;
      url.hash = '';
      const dash = /\.mpd$/i.test(url.pathname);
      const hls = /\.m3u8$/i.test(url.pathname);
      if (
        !observed &&
        !hls &&
        !dash &&
        !/\.(mp4|webm|m4a|mp3|ogg|wav|aac)$/i.test(url.pathname)
      )
        return;
      if (seen.has(url.href)) return;
      seen.add(url.href);
      const name = url.pathname.split('/').at(-1)?.slice(0, 80) || 'media';
      result.push({
        url: url.href,
        mode: dash ? 'dash' : hls ? 'hls' : '',
        label: `${dash ? 'DASH candidate' : hls ? 'HLS candidate' : 'Direct media'} · ${url.hostname} · ${name}`,
      });
    } catch {
      /* Malformed page-provided URLs stay in the browser. */
    }
  };
  const elements = document.querySelectorAll('video,audio,source');
  for (let i = 0; i < Math.min(elements.length, 64); i++) {
    const element = elements[i];
    if (!element) continue;
    add(
      element instanceof HTMLMediaElement
        ? element.currentSrc || element.src
        : element.getAttribute('src') || '',
      true,
    );
  }
  const entries = performance.getEntriesByType('resource');
  for (
    let i = entries.length - 1;
    i >= Math.max(0, entries.length - 256);
    i--
  ) {
    const entry = entries[i];
    if (entry) add(entry.name, false);
  }
  return result;
}
