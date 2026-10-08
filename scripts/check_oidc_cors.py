#!/usr/bin/env python3
"""Read-only post-deployment checks for OIDC CORS (GET and OPTIONS only)."""
import argparse
import json
import sys
import urllib.error
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--issuer', required=True, help='Complete issuer including /oidc/<client_id>')
parser.add_argument('--origin', required=True, help='Registered browser origin without trailing slash')
parser.add_argument('--denied-origin', default='https://cors-denied.invalid')
args = parser.parse_args()
issuer = args.issuer.rstrip('/')
failures = []


def check(label, endpoint, method='GET', origin=None, requested_method=None, requested_headers=None, denied=False):
    headers = {'Origin': origin or args.origin}
    if requested_method:
        headers['Access-Control-Request-Method'] = requested_method
    if requested_headers:
        headers['Access-Control-Request-Headers'] = requested_headers
    request = urllib.request.Request(issuer + endpoint, method=method, headers=headers)
    try:
        try:
            response = urllib.request.urlopen(request, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            status = response.status
            allowed = response.headers.get_all('Access-Control-Allow-Origin') or []
            credentials = response.headers.get('Access-Control-Allow-Credentials')
            if denied:
                assert status == 403 and not allowed, f'expected denied preflight, status={status}, ACAO={allowed}'
            else:
                expected = '*' if method == 'GET' else args.origin
                assert status == (200 if method == 'GET' else 204), f'status={status}'
                assert allowed == [expected], f'ACAO={allowed}, expected exactly one {expected!r}'
                assert credentials is None, f'unexpected credentials header: {credentials}'
                if method == 'OPTIONS':
                    methods = {value.strip() for value in response.headers.get('Access-Control-Allow-Methods', '').split(',')}
                    assert requested_method in methods, f'missing allowed method: {requested_method}'
                    permitted = {value.strip().lower() for value in response.headers.get('Access-Control-Allow-Headers', '').split(',')}
                    assert {'authorization', 'content-type'} <= permitted, 'missing allowed request headers'
                    vary = {value.strip().lower() for line in response.headers.get_all('Vary', []) for value in line.split(',')}
                    assert '*' in vary or {'origin', 'access-control-request-method', 'access-control-request-headers'} <= vary, f'incomplete Vary: {vary}'
                    assert response.headers.get('Access-Control-Max-Age') == '300', 'unexpected preflight cache duration'
                else:
                    body = json.load(response)
                    if endpoint.endswith('openid-configuration'):
                        assert body.get('issuer') == issuer, 'issuer mismatch'
                    else:
                        assert isinstance(body.get('keys'), list) and body['keys'], 'JWKS has no public signing keys; use RS256'
            print(f'PASS {label}')
    except Exception as error:
        failures.append(label)
        print(f'FAIL {label}: {error}')


check('Discovery', '/.well-known/openid-configuration')
check('JWKS', '/jwks')
for endpoint, method in [('/token', 'POST'), ('/userinfo', 'GET'), ('/revoke', 'POST'), ('/introspect', 'POST')]:
    check(endpoint + ' preflight', endpoint, 'OPTIONS', requested_method=method, requested_headers='content-type,authorization')
check('unregistered origin', '/token', 'OPTIONS', origin=args.denied_origin, requested_method='POST', requested_headers='authorization', denied=True)
check('unsupported method', '/token', 'OPTIONS', requested_method='DELETE', denied=True)
check('unsupported header', '/token', 'OPTIONS', requested_method='POST', requested_headers='x-unapproved-cors-header', denied=True)
sys.exit(1 if failures else 0)
