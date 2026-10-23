const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const consent = () => ({ approveScopes: true, acceptedAgreementKeys: ['global:terms'] });
const pending = (state = 'consent_required') => ({ state, transactionToken: 'private-transaction', scope: 'openid' });
const done = () => ({ state: 'authenticated', tokens: { access_token: 'access', refresh_token: 'refresh', scope: 'openid' } });
const config = { name: '测试应用', agreements: [{ key: 'global:terms', title: '服务协议', scope: 'global', code: 'terms' }], registerFields: {} };
const flush = () => new Promise((resolve) => setImmediate(resolve));
function setup() {
  const requests = [], storage = new Map();
  let handler = (r) => {
    if (r.url.endsWith('/config')) return config;
    if (r.url.endsWith('/complete')) return done();
    if (r.url.endsWith('/revoke')) return {};
    return pending();
  };
  const wx = {
    login({ success }) { success({ code: 'login-code' }); },
    getStorageSync(key) { return storage.get(key); },
    setStorageSync(key, value) { storage.set(key, value); },
    removeStorageSync(key) { storage.delete(key); },
    request(r) {
      requests.push(r);
      Promise.resolve().then(() => handler(r)).then((data) => {
        r.success(data.http || { statusCode: 200, data: { code: 0, data } });
      }, r.fail);
    },
  };
  const module = { exports: {} };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'iam.js'), 'utf8'), {
    module, wx, require: () => ({ iamOrigin: 'https://iam.test', clientId: 'example' }),
  });
  const iam = module.exports;
  let page;
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../pages/login/index.js'), 'utf8'), {
    wx, require: () => iam, Page: (definition) => { page = definition; },
  });
  page.setData = function (data) { Object.assign(this.data, data); };
  return { iam, page, requests, storage, handle(fn) { handler = fn; } };
}

test('one entry click completes authorization without displaying consent', async () => {
  const { iam, page, requests } = setup();
  const states = [];
  const setData = page.setData;
  page.setData = function (data) { states.push(data.state); setData.call(this, data); };
  page.onLoad();
  await flush();
  assert.equal(requests.some((r) => r.url.endsWith('/login')), false);
  await page.login();
  assert.equal(page.data.authenticated, true);
  assert.equal(states.includes('consent_required'), false);
  assert.equal(requests.filter((r) => r.url.endsWith('/complete')).length, 1);
  assert.equal(iam.tokens().access_token, 'access');
});

test('phone and binding reuse consent and stop for second factor', async () => {
  for (const action of ['phone', 'bind']) {
    const { iam, requests, storage, handle } = setup();
    handle((r) => {
      if (r.url.endsWith('/login')) return pending(action === 'phone' ? 'phone_required' : 'bind_required');
      if (r.url.endsWith('/complete')) return pending('two_factor_required');
      if (r.url.endsWith('/two-factor/verify')) return done();
      return pending();
    });
    await iam.login({ consent: consent() });
    assert.equal((await iam.native(action, {})).state, 'two_factor_required');
    assert.equal(storage.size, 0);
    assert.equal((await iam.native('two-factor/verify', {})).state, 'authenticated');
    assert.equal(requests.filter((r) => r.url.endsWith('/complete')).length, 1);
  }
});

test('profile completion keeps the original agreement snapshot; legacy login stays manual', async () => {
  const { iam, requests, handle } = setup();
  handle((r) => r.url.endsWith('/complete') ? done() : pending('profile_required'));
  const options = { consent: consent() };
  await iam.login(options);
  options.consent.acceptedAgreementKeys.push('application:new');
  assert.equal(requests.some((r) => r.url.endsWith('/complete')), false);
  await iam.complete({ email: 'test@example.com' });
  assert.deepEqual(Array.from(requests.find((r) => r.url.endsWith('/complete')).data.acceptedAgreementKeys), ['global:terms']);
  handle(() => pending());
  assert.equal((await iam.login()).state, 'consent_required');
  assert.equal(requests.filter((r) => r.url.endsWith('/complete')).length, 1);
});

test('automatic failures reset the page, reload config and never loop or expand consent', async () => {
  for (const failure of ['agreements', 'network', 'scope', 'loop', 'expired']) {
    const { page, requests, storage, handle } = setup();
    handle((r) => {
      if (r.url.endsWith('/config')) return config;
      if (r.url.endsWith('/login')) return failure === 'scope' ? { ...pending(), scope: 'openid phone' } : pending();
      if (failure === 'network') throw new Error('offline');
      if (failure === 'loop') return pending();
      return { http: { statusCode: failure === 'expired' ? 401 : 400, data: { code: 1, message: '请重新确认协议' } } };
    });
    page.onLoad();
    await flush();
    await page.login();
    assert.equal(page.data.state, '');
    assert.ok(page.data.error);
    assert.equal(requests.filter((r) => r.url.endsWith('/config')).length, 2);
    assert.equal(requests.filter((r) => r.url.endsWith('/complete')).length, failure === 'scope' ? 0 : 1);
    assert.equal(storage.size, 0);
  }
});

test('unload and replaced flows cannot persist late credentials or reuse consent', async () => {
  for (const action of ['unload', 'replace']) {
    const { iam, page, storage, handle, requests } = setup();
    let resolve;
    const response = new Promise((r) => { resolve = r; });
    handle((r) => r.url.endsWith('/config') ? config : r.url.endsWith('/complete') ? response : pending());
    page.onLoad();
    await flush();
    const login = page.login();
    await flush();
    assert.equal(requests.filter((r) => r.url.endsWith('/complete')).length, 1);
    if (action === 'unload') page.onUnload();
    else await iam.login();
    resolve(done());
    await login;
    assert.equal(storage.size, 0);
    assert.equal(page.data.authenticated, false);
    assert.equal(requests.some((r) => r.url.endsWith('/revoke')), true);
  }
});

test('native page uses expressions for every conditional', () => {
  const wxml = fs.readFileSync(path.join(__dirname, '../pages/login/index.wxml'), 'utf8');
  for (const match of wxml.matchAll(/wx:if="([^"]+)"/g)) assert.match(match[1], /^\{\{.+\}\}$/);
});
