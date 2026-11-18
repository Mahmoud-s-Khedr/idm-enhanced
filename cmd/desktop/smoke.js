// Test-only driver embedded by the desktop_smoke build tag. It interacts with
// React controls and uses Wails' documented runtime event transport for results.
(async () => {
  const wait = async (read) => {
    const deadline = Date.now() + 20000;
    while (Date.now() < deadline) {
      const value = read();
      if (value) return value;
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    throw new Error('Timed out waiting for a desktop control or state');
  };
  const button = (label, root = document) =>
    [...root.querySelectorAll('button')].find(
      (element) => element.textContent === label && !element.disabled,
    );
  const fill = (selector, value) => {
    const input = document.querySelector(selector);
    const setter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      'value',
    ).set;
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
  };
  let result = 'passed';
  try {
    await wait(() => button('Settings'));
    button('Settings').click();
    await wait(() => document.querySelector('input[type="checkbox"]'));
    // Native delivery is checked separately; no OS permission prompt in smoke.
    document.querySelector('input[type="checkbox"]').click();
    button('Save settings').click();
    await wait(() => document.body.textContent.includes('Settings saved.'));
    button('All downloads').click();
    await wait(() => document.querySelector('input[type="url"]'));
    fill('input[type="url"]', window.__m4Smoke.url);
    fill(
      'input[placeholder="Full path to a new file"]',
      window.__m4Smoke.destination,
    );
    [...document.querySelectorAll('summary')]
      .find((e) => e.textContent === 'Schedule and verification')
      .click();
    fill(
      'input[placeholder="64 hexadecimal characters"]',
      window.__m4Smoke.checksum,
    );
    button('Add to queue').click();
    await wait(() => document.querySelector('article'));
    button('Run queue').click();
    await wait(() => document.querySelector('.badge.downloading'));
    button('Pause all').click();
    await wait(() => document.querySelector('.badge.paused'));
    await wait(() => button('Resume / retry'));
    button('Resume / retry').click();
    await wait(() => document.querySelector('.badge.completed'));
    button('History').click();
    await wait(() => button('Open file'));
    if (!document.body.textContent.includes('8.00 MiB of 8.00 MiB'))
      throw new Error('Incorrect completed progress');
    button('Remove history').click();
    await wait(() => !document.querySelector('article'));
  } catch (error) {
    result = String(error);
  }
  await fetch('/wails/runtime', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'x-wails-client-id': 'desktop-smoke',
    },
    body: JSON.stringify({
      object: 3,
      method: 0,
      args: { name: 'm4:smoke', data: result },
    }),
  });
})();
