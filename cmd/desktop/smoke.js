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
    throw new Error(
      `Timed out waiting for ${String(read)}; status: ${[...document.querySelectorAll('[role="status"]')].map((element) => element.textContent).join('; ')}`,
    );
  };
  const button = (label, root = document) =>
    [...root.querySelectorAll('button')].find(
      (element) => element.textContent === label && !element.disabled,
    );
  const fill = (selector, value) => {
    const input = document.querySelector(selector);
    const setter = Object.getOwnPropertyDescriptor(
      input instanceof HTMLSelectElement
        ? HTMLSelectElement.prototype
        : HTMLInputElement.prototype,
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
    await wait(() =>
      document.querySelector(
        'section[aria-labelledby="settings-title"] input[type="checkbox"]',
      ),
    );
    // Native delivery is checked separately; no OS permission prompt in smoke.
    document
      .querySelector(
        'section[aria-labelledby="settings-title"] input[type="checkbox"]',
      )
      .click();
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
    button('Cancel…').click();
    await wait(() => document.activeElement === button('Keep partial'));
    button('Keep partial').dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    );
    await wait(() => !button('Keep partial'));
    if (document.activeElement !== button('Cancel…'))
      throw new Error('Cancel dismissal lost keyboard focus');
    document.documentElement.style.fontSize = '200%';
    // Reading scrollWidth forces current layout even when the compositor has
    // stopped animation frames for an occluded window.
    if (document.documentElement.scrollWidth > window.innerWidth)
      throw new Error('Download controls overflow at 200% text size');
    document.documentElement.style.fontSize = '';
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
    button('All downloads').click();
    await wait(() =>
      document.querySelector(
        'section[aria-labelledby="add-title"] input[type="checkbox"]',
      ),
    );
    document
      .querySelector(
        'section[aria-labelledby="add-title"] input[type="checkbox"]',
      )
      .click();
    await wait(() =>
      [...document.querySelectorAll('button')].some(
        (element) => element.textContent === 'Analyze HLS',
      ),
    );
    fill('input[type="url"]', window.__m4Smoke.mediaURL);
    fill(
      'input[placeholder="Full path to a new file"]',
      window.__m4Smoke.mediaDestination,
    );
    await wait(() => button('Analyze HLS'));
    button('Analyze HLS').click();
    await wait(
      () => document.querySelector('select') && button('Add to queue'),
    );
    const quality = document.querySelector('select').options[1].value;
    fill('select', quality);
    document.documentElement.style.fontSize = '200%';
    if (document.documentElement.scrollWidth > window.innerWidth)
      throw new Error('HLS quality form overflows at 200%');
    document.documentElement.style.fontSize = '';
    button('Add to queue').click();
    await wait(() => document.querySelector('article'));
    button('Run queue').click();
    await wait(() => document.querySelector('.badge.completed'));
    if (!document.body.textContent.includes('2 of 2 segments'))
      throw new Error('Incorrect media resource progress');
    button('History').click();
    await wait(() => button('Remove history'));
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
