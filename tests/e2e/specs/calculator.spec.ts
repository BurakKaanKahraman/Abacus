import { expect, test, type Page } from '@playwright/test';

/**
 * Acceptance tests for the critical user journeys, driven through a real
 * browser against the running stack. Nothing is stubbed: every calculation
 * here is answered by the Go engine.
 */

/** Clicks keypad buttons by their accessible name. */
async function press(page: Page, ...names: string[]): Promise<void> {
  for (const name of names) {
    await page.getByRole('button', { name, exact: true }).click();
  }
}

/** Enters an expression using the keypad, character by character. */
async function type(page: Page, expression: string): Promise<void> {
  const keyNames: Record<string, string> = {
    '+': 'Add',
    '-': 'Subtract',
    '*': 'Multiply',
    '/': 'Divide',
    '^': 'Power',
    '%': 'Modulo',
    '.': 'Decimal point',
  };

  for (const character of expression.replace(/\s/g, '')) {
    await press(page, keyNames[character] ?? character);
  }
}

test.beforeEach(async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1, name: 'Abacus' })).toBeVisible();
});

test.describe('calculating', () => {
  test('applies operator precedence across a complex expression', async ({ page }) => {
    await type(page, '10+20*3-15/(5-2)');

    // The preview is computed in the browser before anything is submitted.
    await expect(page.getByTestId('preview')).toContainText('65');

    await press(page, 'Calculate');

    // And the answer comes back from the Go engine.
    await expect(page.getByTestId('result')).toHaveText('= 65');
  });

  test.describe('assessment scenarios', () => {
    const cases = [
      { expression: '10+20*3', expected: '= 70', note: 'multiplication before addition' },
      { expression: '(10+20)*3', expected: '= 90', note: 'parentheses override precedence' },
      { expression: '2^3^2', expected: '= 512', note: 'power is right associative' },
      { expression: '100%7', expected: '= 2', note: 'modulo shares the multiplicative tier' },
    ];

    for (const { expression, expected, note } of cases) {
      test(`${expression} (${note})`, async ({ page }) => {
        await type(page, expression);
        await press(page, 'Calculate');

        await expect(page.getByTestId('result')).toHaveText(expected);
      });
    }
  });

  test('evaluates a square root inside a larger expression', async ({ page }) => {
    await press(page, '(', '1', '0', 'Add', 'Square root', '1', '6', ')', ')', 'Multiply', '2', 'Power', '3');

    await press(page, 'Calculate');

    await expect(page.getByTestId('result')).toHaveText('= 112');
  });

  test('keeps full precision rather than rounding the answer', async ({ page }) => {
    await type(page, '3+4*2/(1-5)^2^3');

    await press(page, 'Calculate');

    await expect(page.getByTestId('result')).toHaveText('= 3.0001220703125');
  });
});

test.describe('error handling', () => {
  test('reports division by zero from the backend', async ({ page }) => {
    await type(page, '15/(5-5)');

    await press(page, 'Calculate');

    await expect(page.getByTestId('error')).toContainText('Division by zero');
  });

  test('catches unbalanced parentheses before contacting the server', async ({ page }) => {
    const requests: string[] = [];
    page.on('request', (request) => {
      if (request.url().includes('/calculate')) requests.push(request.url());
    });

    await press(page, '(', '1', 'Add', '2');

    await expect(page.getByTestId('syntax-hint')).toContainText('Missing 1 closing parenthesis');

    await press(page, 'Calculate');

    await expect(page.getByTestId('error')).toContainText('Missing 1 closing parenthesis');
    expect(requests, 'an invalid expression must never reach the API').toHaveLength(0);
  });

});

test.describe('history', () => {
  test('records a calculation and restores it on click', async ({ page }) => {
    await type(page, '6*7');
    await press(page, 'Calculate');
    await expect(page.getByTestId('result')).toHaveText('= 42');

    const history = page.getByRole('region', { name: 'Calculation history' });
    await expect(history.getByRole('button', { name: /Reuse/ })).toBeVisible();

    await press(page, 'Clear all');
    await expect(page.getByTestId('expression')).toHaveText('0');

    await history.getByRole('button', { name: /Reuse/ }).click();

    await expect(page.getByTestId('expression')).toHaveText('6×7');
  });

  test('survives a page reload', async ({ page }) => {
    await type(page, '8*8');
    await press(page, 'Calculate');
    await expect(page.getByTestId('result')).toHaveText('= 64');

    await page.reload();

    await expect(page.getByRole('region', { name: 'Calculation history' })).toContainText('64');
  });
});

test.describe('keyboard', () => {
  test('drives the whole calculation from the keyboard', async ({ page }) => {
    await page.keyboard.type('10+20*3');
    await expect(page.getByTestId('preview')).toContainText('70');

    await page.keyboard.press('Enter');
    await expect(page.getByTestId('result')).toHaveText('= 70');

    await page.keyboard.press('Escape');
    await expect(page.getByTestId('expression')).toHaveText('0');
  });
});

test.describe('preview mode', () => {
  /** Counts requests the page makes to the calculate endpoint. */
  function countCalculateRequests(page: Page): () => number {
    let count = 0;
    page.on('request', (request) => {
      if (request.url().includes('/calculate')) count += 1;
    });
    return () => count;
  }

  test('previews in the browser by default, with no network traffic', async ({ page }) => {
    const calls = countCalculateRequests(page);

    await type(page, '10+20*3');

    await expect(page.getByTestId('preview')).toContainText('70');
    expect(calls()).toBe(0);
    await expect(page.getByRole('switch', { name: /Server preview/ })).toHaveAttribute(
      'aria-checked',
      'false',
    );
  });

  test('asks the server for the preview once switched on', async ({ page }) => {
    const calls = countCalculateRequests(page);

    await page.getByRole('switch', { name: /Server preview/ }).click();
    await type(page, '10+20*3');

    await expect(page.getByTestId('preview')).toContainText('70');
    expect(calls(), 'the preview must come from the API in remote mode').toBeGreaterThan(0);
  });

  // The whole reason the grammar exists twice is that the two must agree.
  // Switching modes on the same expression is the most direct check there is.
  test('both modes agree on the same expression', async ({ page }) => {
    await type(page, '10+20*3-15/(5-2)');
    await expect(page.getByTestId('preview')).toContainText('65');

    await page.getByRole('switch', { name: /Server preview/ }).click();

    await expect(page.getByTestId('preview')).toContainText('65');
  });

  test('remembers the choice across a reload', async ({ page }) => {
    await page.getByRole('switch', { name: /Server preview/ }).click();
    await expect(page.getByRole('switch')).toHaveAttribute('aria-checked', 'true');

    await page.reload();

    await expect(page.getByRole('switch', { name: /Server preview/ })).toHaveAttribute(
      'aria-checked',
      'true',
    );
  });

  test('still refuses to send a syntactically invalid expression', async ({ page }) => {
    const calls = countCalculateRequests(page);

    await page.getByRole('switch', { name: /Server preview/ }).click();
    await press(page, '(', '1', 'Add', '2');

    await expect(page.getByTestId('syntax-hint')).toContainText('Missing 1 closing parenthesis');
    expect(calls()).toBe(0);
  });
});

test.describe('theme', () => {
  // The browser reports a colour scheme, and the app follows it until the user
  // says otherwise, so the preference is pinned here rather than assumed.
  test.use({ colorScheme: 'dark' });

  test('starts from the system preference', async ({ page }) => {
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  });

  test('switches and remembers the palette', async ({ page }) => {
    const root = page.locator('html');
    await expect(root).toHaveAttribute('data-theme', 'dark');

    await page.getByRole('button', { name: 'Switch to light theme' }).click();
    await expect(root).toHaveAttribute('data-theme', 'light');

    await page.reload();

    await expect(root).toHaveAttribute('data-theme', 'light');
  });
});

test.describe('theme on a light system', () => {
  test.use({ colorScheme: 'light' });

  test('follows a light system preference without being told', async ({ page }) => {
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  });
});

test.describe('delivery', () => {
  test('serves the application with its security headers', async ({ page }) => {
    const response = await page.goto('/');

    expect(response?.status()).toBe(200);

    const headers = response?.headers() ?? {};
    expect(headers['x-content-type-options']).toBe('nosniff');
    expect(headers['x-frame-options']).toBe('DENY');
    expect(headers['content-security-policy']).toContain("default-src 'self'");
    expect(headers['referrer-policy']).toBe('strict-origin-when-cross-origin');
  });

  test('reaches the API on the same origin, so no CORS preflight is needed', async ({ page }) => {
    const preflights: string[] = [];
    page.on('request', (request) => {
      if (request.method() === 'OPTIONS') preflights.push(request.url());
    });

    await type(page, '1+1');
    await press(page, 'Calculate');
    await expect(page.getByTestId('result')).toHaveText('= 2');

    expect(preflights).toHaveLength(0);
  });
});
