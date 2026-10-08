"""Browser regression driver for TestOIDCCORSBrowserFlow.

Requires Python Playwright and Chromium. Receives only an ephemeral local test
fixture on stdin; never logs the authorization code or access token.
"""
import json
import os
import sys
from playwright.sync_api import sync_playwright

fixture = json.load(sys.stdin)
with sync_playwright() as playwright:
    options = {"headless": True}
    if executable := os.environ.get("OIDC_BROWSER_EXECUTABLE"):
        options["executable_path"] = executable
    browser = playwright.chromium.launch(**options)
    try:
        page = browser.new_page()
        page.goto(fixture["rp_url"])
        page.wait_for_load_state("networkidle")
        result = page.evaluate("""async (fixture) => {
          const getJSON = async (url, init = {}) => {
            const response = await fetch(url, { credentials: 'omit', ...init });
            if (!response.ok) throw new Error(`HTTP ${response.status}: ${url}`);
            return response.json();
          };
          const metadata = await getJSON(fixture.issuer + '/.well-known/openid-configuration');
          if (metadata.issuer !== fixture.issuer) throw new Error('Wrong discovery issuer');
          const jwks = await getJSON(metadata.jwks_uri);
          const tokens = await getJSON(metadata.token_endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'X-Trace-Id': 'browser-cors-regression' },
            body: new URLSearchParams({ grant_type: 'authorization_code', client_id: fixture.client_id,
              redirect_uri: fixture.redirect_uri, code: fixture.authorization_code, code_verifier: fixture.code_verifier })
          });
          const decode = (value) => Uint8Array.from(atob(value.replace(/-/g, '+').replace(/_/g, '/')), c => c.charCodeAt(0));
          const parts = tokens.access_token.split('.');
          const header = JSON.parse(new TextDecoder().decode(decode(parts[0])));
          const claims = JSON.parse(new TextDecoder().decode(decode(parts[1])));
          const key = jwks.keys.find(key => key.kid === header.kid);
          if (!key || header.typ !== 'at+jwt' || header.alg !== 'RS256') throw new Error('Wrong JWT header/JWKS');
          const cryptoKey = await crypto.subtle.importKey('jwk', key, { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' }, false, ['verify']);
          const verified = await crypto.subtle.verify('RSASSA-PKCS1-v1_5', cryptoKey, decode(parts[2]), new TextEncoder().encode(parts[0] + '.' + parts[1]));
          if (!verified || claims.iss !== fixture.issuer || claims.aud !== fixture.client_id || claims.exp <= Date.now()/1000) throw new Error('JWT validation failed');
          const user = await getJSON(metadata.userinfo_endpoint, { headers: { Authorization: 'Bearer ' + tokens.access_token } });
          if (user.sub !== claims.sub) throw new Error('Wrong UserInfo subject');
          let credentialedMetadataBlocked = false;
          try { await fetch(fixture.issuer + '/.well-known/openid-configuration', { credentials: 'include' }); }
          catch (error) { credentialedMetadataBlocked = error instanceof TypeError; }
          return { discovery: true, jwks: true, token: true, userinfo: true, signature: verified,
            credentialed_metadata_blocked: credentialedMetadataBlocked,
            access_token: tokens.access_token, userinfo_endpoint: metadata.userinfo_endpoint };
        }""", fixture)
        assert result["credentialed_metadata_blocked"]
        page.goto(fixture["foreign_url"])
        page.wait_for_load_state("networkidle")
        blocked = page.evaluate("""async (result) => {
          try {
            await fetch(result.userinfo_endpoint, { credentials: 'omit', headers: { Authorization: 'Bearer ' + result.access_token } });
            return false;
          } catch (error) { return error instanceof TypeError; }
        }""", result)
        assert blocked, "Unregistered origin must not read UserInfo"
        result.pop("access_token")
        result.pop("userinfo_endpoint")
        result["foreign_origin_blocked"] = blocked
        print(json.dumps(result))
    finally:
        browser.close()
