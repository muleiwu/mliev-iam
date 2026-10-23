const { iamOrigin, clientId } = require('../config');
const storageKey = `iam:${clientId}:tokens`;
let flow; // Transaction and entry consent stay in memory, scoped to one login.
let refreshPending;
const normalizedScope = (scope) => [...new Set(scope.trim().split(/\s+/).filter(Boolean))].sort().join(' ');
function cancelLogin() { flow = undefined; }
function assertCurrent(current) {
  if (!current || current !== flow) {
    const error = new Error('本次登录已取消，请重新开始');
    error.cancelled = true;
    throw error;
  }
}
function resetFailedLogin(current, error) {
  if (flow === current) { cancelLogin(); error.restartLogin = true; }
  return error;
}
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
async function discardTokens(tokens) {
  if (!tokens) return;
  await request(`/oidc/${encodeURIComponent(clientId)}/revoke`, 'POST', {
    client_id: clientId, token: tokens.refresh_token || tokens.access_token,
    token_type_hint: tokens.refresh_token ? 'refresh_token' : 'access_token',
  }, { 'content-type': 'application/x-www-form-urlencoded' }).catch(() => {});
}
async function native(action, data, current = flow) {
  assertCurrent(current);
  const result = await request(`/api/auth/miniprogram/${encodeURIComponent(clientId)}/${action}`, 'POST', data,
    current.token ? { Authorization: `Bearer ${current.token}` } : {});
  if (flow !== current) await discardTokens(result.tokens);
  assertCurrent(current);
  if (current.consent) {
    const scope = result.state === 'authenticated' ? result.tokens && result.tokens.scope : result.scope;
    if (typeof scope !== 'string' || normalizedScope(scope) !== current.scope) {
      const error = resetFailedLogin(current, new Error('授权范围发生变化，请重新确认后登录'));
      await discardTokens(result.tokens);
      throw error;
    }
  }
  if (result.transactionToken) current.token = result.transactionToken;
  if (result.state === 'consent_required' && current.consent) {
    try {
      if (action === 'complete' || current.autoConsentUsed) throw new Error('应用授权未完成，请重新登录');
      current.autoConsentUsed = true;
      return await native('complete', current.consent, current);
    } catch (error) { throw resetFailedLogin(current, error); }
  }
  if (result.state === 'authenticated') { cancelLogin(); saveTokens(result.tokens); }
  return result;
}
async function login(options = {}) {
  const scope = options.scope || 'openid';
  if (options.consent !== undefined && (!options.consent || options.consent.approveScopes !== true ||
    !Array.isArray(options.consent.acceptedAgreementKeys) || options.consent.acceptedAgreementKeys.some((key) => typeof key !== 'string' || !key.trim())))
    throw new Error('请明确确认授权范围和协议');
  const current = flow = { scope: normalizedScope(scope), token: '', consent: options.consent ? {
    approveScopes: true, acceptedAgreementKeys: [...options.consent.acceptedAgreementKeys],
  } : undefined };
  try {
    const code = await new Promise((resolve, reject) => wx.login({
      success: (res) => res.code ? resolve(res.code) : reject(new Error('无法取得微信登录凭证')),
      fail: () => reject(new Error('微信登录失败')),
    }));
    assertCurrent(current);
    return await native('login', { jsCode: code, scope, nonce: options.nonce }, current);
  } catch (error) { throw resetFailedLogin(current, error); }
}
function complete(profile) {
  if (!flow || !flow.consent) return Promise.reject(new Error('请先在入口确认协议并登录'));
  return native('complete', { ...flow.consent, profile });
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
  cancelLogin();
  const current = tokens();
  if (current && current.refresh_token) await request(`/oidc/${encodeURIComponent(clientId)}/revoke`, 'POST', {
    client_id: clientId, token: current.refresh_token, token_type_hint: 'refresh_token',
  }, { 'content-type': 'application/x-www-form-urlencoded' });
  wx.removeStorageSync(storageKey);
}
async function agreement(link) {
  const path = link.scope === 'global'
    ? `/api/agreements/global/${encodeURIComponent(link.code)}`
    : `/api/agreements/application?client_id=${encodeURIComponent(clientId)}&code=${encodeURIComponent(link.code)}`;
  return request(path, 'GET');
}
module.exports = { login, native, complete, cancelLogin, config, tokens, refresh, logout, agreement };
