const test = require('node:test');
const assert = require('node:assert/strict');

test('native continuation, single-flight refresh, and logout', async () => {
  const storage = new Map();
  const requests = [];
  let failLogout = false;
  global.wx = {
    login({ success }) { success({ code: 'login-code' }); },
    getStorageSync(key) { return storage.get(key); },
    setStorageSync(key, value) { storage.set(key, value); },
    removeStorageSync(key) { storage.delete(key); },
    request(options) {
      requests.push(options);
      let data;
      if (options.url.endsWith('/login')) data = { code: 0, data: { state: 'phone_required', transactionToken: 'transaction' } };
      else if (options.url.endsWith('/phone')) data = { code: 0, data: { state: 'profile_required', transactionToken: 'transaction' } };
      else if (options.url.endsWith('/complete')) data = { code: 0, data: { state: 'authenticated', tokens: { access_token: 'access', refresh_token: 'refresh' } } };
      else if (options.url.endsWith('/token')) data = { access_token: 'new-access', refresh_token: 'new-refresh' };
      else if (failLogout) { options.fail(); return; }
      else data = {};
      setImmediate(() => options.success({ statusCode: 200, data }));
    },
  };
  const iam = require('./iam');
  await iam.login();
  assert.equal(requests[0].data.jsCode, 'login-code');
  await iam.native('phone', { phoneCode: 'phone-code' });
  assert.equal(requests[1].header.Authorization, 'Bearer transaction');
  assert.equal(requests[1].data.phoneCode, 'phone-code');
  await iam.native('complete', { approveScopes: true });
  await Promise.all([iam.refresh(), iam.refresh()]);
  assert.equal(requests.filter((r) => r.url.endsWith('/token')).length, 1);
  assert.equal(iam.tokens().refresh_token, 'new-refresh');
  failLogout = true;
  await assert.rejects(iam.logout());
  assert.equal(iam.tokens().refresh_token, 'new-refresh', 'keep token available for revocation retry');
  failLogout = false;
  await iam.logout();
  assert.equal(iam.tokens(), undefined);
  assert.equal(requests.at(-1).data.token, 'new-refresh');
  assert.equal(requests.some((r) => JSON.stringify(r.data).includes('client_secret')), false);
});
