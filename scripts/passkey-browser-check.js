// Run with playwright-cli run-code --filename=scripts/passkey-browser-check.js
// after opening the isolated TestPasskeyBrowser fixture's /auth/#/account URL.
async page => {
  const base = page.url().split('/').slice(0, 3).join('/');
  // Each fixture has a fresh database; discard cookies from an earlier run.
  await page.context().clearCookies();
  await page.goto(`${base}/auth/#/account`);
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('WebAuthn.disable');
  await cdp.send('WebAuthn.enable');
  const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: { protocol: 'ctap2', transport: 'internal', hasResidentKey: true, hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true },
  });
  const steps = [];
  const post = async (path, body = {}) => page.evaluate(async ({ path, body }) => {
    const response = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    return { status: response.status, body: await response.json() };
  }, { path, body });
  const security = async () => {
    await page.goto(`${base}/auth/#/account?tab=security`);
    await page.getByRole('tab', { name: '安全设置', exact: true }).click();
    await page.getByRole('button', { name: '添加 Passkey', exact: true }).waitFor();
  };
  const verifyPassword = async () => {
    const choice = page.getByRole('button', { name: '账户密码', exact: true });
    await choice.waitFor();
    if (await page.locator('input[id^="two-factor-code-"]').count()) throw new Error('Verification input appeared before choosing a method');
    await choice.click();
    await page.locator('input[id^="two-factor-code-"]').fill('correct-password');
    await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  };
  const logout = async () => {
    const response = await post('/api/oidc/auth/logout');
    if (response.status !== 200) throw new Error(`logout failed ${JSON.stringify(response)}`);
    await page.goto(`${base}/auth/#/account/login`);
    await page.waitForURL(`${base}/auth/#/oidc/authorize?**`);
    await page.getByRole('button', { name: '使用 Passkey 登录' }).waitFor();
  };
  const passwordLogin = async () => {
    await page.getByRole('tab', { name: '账户', exact: true }).click();
    await page.getByLabel('账户', { exact: true }).fill('passkey-user');
    await page.getByRole('button', { name: '继续', exact: true }).click();
    await page.getByLabel('密码', { exact: true }).fill('correct-password');
    await page.getByRole('button', { name: '登录', exact: true }).click();
  };
  await passwordLogin();
  await page.waitForURL('**/auth/#/account');
  await security();
  await page.getByRole('button', { name: '添加 Passkey', exact: true }).click();
  await page.getByLabel('为新 Passkey 命名').fill('Browser test Passkey');
  await page.getByRole('button', { name: '确认身份并继续' }).click();
  await verifyPassword();
  await page.getByText('Browser test Passkey', { exact: true }).waitFor();
  steps.push('Register discoverable Passkey with user verification');
  await logout();
  await page.getByRole('button', { name: '使用 Passkey 登录' }).click();
  await page.waitForURL('**/auth/#/account');
  steps.push('Username-less Passkey login');
  await security();
  await page.getByRole('button', { name: '开启两步验证', exact: true }).click();
  await verifyPassword();
  await page.getByRole('button', { name: '我已保存' }).waitFor();
  await page.getByRole('button', { name: '我已保存' }).click();
  steps.push('Enable two-step verification separately');
  await logout();
  await page.getByRole('button', { name: '使用 Passkey 登录' }).click();
  await page.waitForURL('**/auth/#/two-factor?**');
  if (await page.getByRole('button', { name: 'Passkey', exact: true }).count()) throw new Error('Primary Passkey was offered a second time');
  await verifyPassword();
  await page.waitForURL('**/auth/#/account');
  steps.push('Passkey then password');
  await logout();
  await passwordLogin();
  await page.waitForURL('**/auth/#/two-factor?**');
  if (await page.getByRole('button', { name: '账户密码', exact: true }).count()) throw new Error('Primary password was offered a second time');
  await page.getByRole('button', { name: 'Passkey', exact: true }).click();
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.waitForURL('**/auth/#/account');
  steps.push('Password then Passkey');
  await logout();
  const secondFactorTransaction = await page.evaluate(async () => (await (await fetch('/__test/oidc')).json()).data);
  await page.goto(`${base}/auth/#/oidc/login?session_id=${secondFactorTransaction.sessionId}`);
  await page.getByRole('button', { name: '使用 Passkey 登录' }).click();
  await page.waitForURL('**/auth/#/two-factor?**');
  if (await page.getByRole('button', { name: 'Passkey', exact: true }).count()) throw new Error('OIDC offered the primary Passkey again');
  await verifyPassword();
  await page.waitForURL(url => !url.hash.startsWith('#/two-factor'));
  const secondFactorFlow = await page.evaluate(async id => (await (await fetch(`/api/oidc/flow-status?session_id=${id}`)).json()), secondFactorTransaction.sessionId);
  if (secondFactorFlow.code !== 0 || ['needs_login', 'needs_two_factor', 'error'].includes(secondFactorFlow.data.status)) throw new Error(`OIDC two-factor not authenticated ${JSON.stringify(secondFactorFlow)}`);
  steps.push('OIDC Passkey then password with explicit method selection');
  await security();
  await page.getByRole('button', { name: '关闭两步验证', exact: true }).click();
  await page.getByRole('checkbox').check();
  await verifyPassword();
  await page.getByRole('button', { name: '开启两步验证', exact: true }).waitFor();
  await page.getByText('Browser test Passkey', { exact: true }).waitFor();
  steps.push('Disabling two-step verification preserves Passkey');
  await page.getByRole('button', { name: '重命名 Browser test Passkey', exact: true }).click();
  await page.getByLabel('重命名 Passkey', { exact: true }).fill('Renamed Passkey');
  await page.getByRole('button', { name: '保存名称' }).click();
  await page.getByText('Renamed Passkey', { exact: true }).waitFor();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: '/tmp/iam-passkey-mobile.png', fullPage: true });
  if (await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)) throw new Error('Mobile overflow');
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.screenshot({ path: '/tmp/iam-passkey-desktop.png', fullPage: true });
  steps.push('Rename, mobile and desktop layouts');
  await logout();
  const transaction = await page.evaluate(async () => (await (await fetch('/__test/oidc')).json()).data);
  await page.goto(`${base}/auth/#/oidc/login?session_id=${transaction.sessionId}`);
  await page.getByRole('button', { name: '使用 Passkey 登录' }).click();
  await page.waitForURL(url => !url.hash.startsWith('#/oidc/login'));
  const flow = await page.evaluate(async id => (await (await fetch(`/api/oidc/flow-status?session_id=${id}`)).json()), transaction.sessionId);
  if (flow.code !== 0 || ['needs_login', 'needs_two_factor', 'error'].includes(flow.data.status)) throw new Error(`OIDC not authenticated ${JSON.stringify(flow)}`);
  steps.push('OIDC Passkey login');
  await logout();
  // The parent is deliberately a different origin. The explicit link must escape the frame.
  await page.goto(`${base.replace('localhost', '127.0.0.1')}/__test/iframe`);
  const frame = page.frameLocator('iframe');
  await frame.getByRole('link', { name: '打开完整认证页面' }).click();
  await page.waitForURL(`${base}/auth/#/oidc/authorize?**`);
  await page.getByRole('button', { name: '使用 Passkey 登录' }).click();
  await page.waitForURL('**/auth/#/account');
  steps.push('Cross-origin iframe opens the full authentication page');
  await security();
  await page.getByRole('button', { name: '删除 Renamed Passkey', exact: true }).click();
  await page.getByRole('button', { name: '确认身份并继续' }).click();
  await verifyPassword();
  await page.getByText('尚未添加 Passkey。添加后，登录时无需输入用户名和密码。', { exact: true }).waitFor();
  steps.push('Delete credential with reauthentication');
  await cdp.send('WebAuthn.removeVirtualAuthenticator', { authenticatorId });
  return steps;
}
