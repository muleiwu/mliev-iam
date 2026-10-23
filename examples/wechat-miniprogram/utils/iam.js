const { iamOrigin, clientId } = require('../config');
const storageKey = `iam:${clientId}:tokens`;
let continuation = ''; // Memory only. Restart login when the app is relaunched.
let refreshPending;
function request(path, method, data, header = {}) {
  return new Promise((resolve, reject) => wx.request({
    url: iamOrigin + path, method, data, header, timeout: 15000,
    success(res) {
      const body = res.data;
      if (res.statusCode >= 400 || (body && typeof body.code === 'number' && body.code !== 0) || (body && body.error)) {
        const error = new Error((body && (body.message || body.error_description)) || '请求失败');
        error.status = res.statusCode;
        reject(error);
      } else resolve(body && typeof body.code === 'number' ? body.data : body);
    },
    fail() { reject(new Error('网络请求失败。若正在进行微信登录或手机号授权，请重新获取凭证。')); },
  }));
}
function saveTokens(tokens) {
  wx.setStorageSync(storageKey, tokens);
}
async function native(action, data) {
  const result = await request(`/api/auth/miniprogram/${encodeURIComponent(clientId)}/${action}`, 'POST', data,
    continuation ? { Authorization: `Bearer ${continuation}` } : {});
  if (result.transactionToken) continuation = result.transactionToken;
  if (result.state === 'authenticated') { continuation = ''; saveTokens(result.tokens); }
  return result;
}
async function login() {
  continuation = '';
  const code = await new Promise((resolve, reject) => wx.login({
    success: (res) => res.code ? resolve(res.code) : reject(new Error('无法取得微信登录凭证')),
    fail: () => reject(new Error('微信登录失败')),
  }));
  return native('login', { jsCode: code, scope: 'openid' });
}
function config() { return request(`/api/auth/miniprogram/${encodeURIComponent(clientId)}/config`, 'GET'); }
function tokens() { return wx.getStorageSync(storageKey); }
async function refresh() {
  // All callers share one refresh attempt; rotating refresh tokens are single use.
  if (refreshPending) return refreshPending;
  refreshPending = (async () => {
    const current = tokens();
    if (!current || !current.refresh_token) throw new Error('请先登录');
    const result = await request(`/oidc/${encodeURIComponent(clientId)}/token`, 'POST', {
      client_id: clientId, grant_type: 'refresh_token', refresh_token: current.refresh_token,
    }, { 'content-type': 'application/x-www-form-urlencoded' });
    saveTokens(result);
    return result;
  })();
  try { return await refreshPending; } finally { refreshPending = undefined; }
}
async function logout() {
  const current = tokens();
  if (current && current.refresh_token) await request(`/oidc/${encodeURIComponent(clientId)}/revoke`, 'POST', {
    client_id: clientId, token: current.refresh_token, token_type_hint: 'refresh_token',
  }, { 'content-type': 'application/x-www-form-urlencoded' });
  wx.removeStorageSync(storageKey);
  continuation = '';
}
async function agreement(link) {
  const path = link.scope === 'global'
    ? `/api/agreements/global/${encodeURIComponent(link.code)}`
    : `/api/agreements/application?client_id=${encodeURIComponent(clientId)}&code=${encodeURIComponent(link.code)}`;
  return request(path, 'GET');
}
module.exports = { login, native, config, tokens, refresh, logout, agreement };
