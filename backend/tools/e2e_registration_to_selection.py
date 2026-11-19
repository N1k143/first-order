#!/usr/bin/env python3
"""Explicitly gated production API smoke test; never runs during ordinary tests.

Required environment:
  E2E_TARGET_URL=https://your-application.example
  E2E_ALLOW_WRITES=1
  E2E_RUN_TAG=foe2e-YYYYMMDD-yourtag (must contain 'e2e')
  E2E_COMPANY_EMAIL / E2E_COMPANY_PASSWORD
  E2E_DEVELOPER_EMAIL / E2E_DEVELOPER_PASSWORD
Both email local-parts must contain E2E_RUN_TAG. Passwords stay in memory.

Optional E2E_REGISTER=1 creates these two accounts via the public registration
endpoint, with a tagged NEW company. It additionally requires E2E_EMAIL_POLICY
set to 'autoconfirm' or 'confirmation', matching the operator-approved Auth
configuration. Sending signup email requires E2E_ALLOW_EMAIL=1 and addresses
explicitly supplied by the operator. This runner never changes Auth policy.
For confirmation-required accounts, confirm via the delivered email and rerun
with E2E_REGISTER=0; alternatively provide E2E_{COMPANY,DEVELOPER}_TOKEN_HASH
and E2E_CONFIRMATION_SOURCE=delivered_email. A separately approved staged test
may use admin-generated signup hashes with E2E_CONFIRMATION_SOURCE=admin_generate_link
and E2E_ALLOW_EMAIL_BYPASS=1. That mode MUST use E2E_REGISTER=0: the operator
provisions the two tagged accounts separately, no email is sent by this runner,
and its report explicitly states that public signup and email delivery were not
tested. It cannot be reported as a successful registration/email test.
Existing-account mode is the default. No resend/forgot-password calls are made.

Writes only the tagged accounts/company/order/application. A mode-0600 JSON
manifest records IDs, tagged email addresses, statuses and request IDs for
reviewed cleanup. It NEVER records or prints passwords, cookies or tokens.
No automatic cleanup/deletion is performed. HTTP is allowed only on loopback
with E2E_ALLOW_HTTP=1. No redirects, insecure TLS or cross-origin requests.
"""
import datetime as dt
import http.cookiejar
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path


class Stop(Exception):
    def __init__(self, code, step='', status=None):
        self.code, self.step, self.status = code, step, status
        super().__init__(code)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def required(key):
    value = os.environ.get(key, '')
    if not value:
        raise Stop('MISSING_' + key)
    return value


def safe_code(value):
    return value if isinstance(value, str) and re.fullmatch(r'[A-Z0-9_]{1,80}', value) else 'UNEXPECTED_RESPONSE'


def main():
    if os.environ.get('E2E_ALLOW_WRITES') != '1':
        raise Stop('WRITES_NOT_AUTHORIZED')
    target = required('E2E_TARGET_URL').rstrip('/')
    parsed = urllib.parse.urlsplit(target)
    if parsed.username or parsed.password or parsed.query or parsed.fragment or parsed.path not in ('', '/'):
        raise Stop('TARGET_MUST_BE_ORIGIN')
    local_http = parsed.scheme == 'http' and parsed.hostname in ('localhost', '127.0.0.1', '::1') and os.environ.get('E2E_ALLOW_HTTP') == '1'
    if not parsed.hostname or (parsed.scheme != 'https' and not local_http):
        raise Stop('HTTPS_TARGET_REQUIRED')
    tag = required('E2E_RUN_TAG')
    if not re.fullmatch(r'[a-z0-9][a-z0-9-]{7,63}', tag) or 'e2e' not in tag:
        raise Stop('INVALID_TEST_TAG')
    register = os.environ.get('E2E_REGISTER') == '1'
    if register and os.environ.get('E2E_EMAIL_POLICY') not in ('autoconfirm', 'confirmation'):
        raise Stop('EXPLICIT_EMAIL_POLICY_REQUIRED')
    if register and os.environ.get('E2E_EMAIL_POLICY') == 'confirmation' and os.environ.get('E2E_ALLOW_EMAIL') != '1':
        raise Stop('EMAIL_SEND_NOT_AUTHORIZED')
    confirmation_source = os.environ.get('E2E_CONFIRMATION_SOURCE', '')
    token_provided = any(os.environ.get('E2E_' + role + '_TOKEN_HASH') for role in ('COMPANY', 'DEVELOPER'))
    if token_provided and confirmation_source not in ('delivered_email', 'admin_generate_link'):
        raise Stop('CONFIRMATION_SOURCE_REQUIRED')
    bypass_email = confirmation_source == 'admin_generate_link'
    if bypass_email and (register or os.environ.get('E2E_ALLOW_EMAIL_BYPASS') != '1' or not token_provided):
        raise Stop('EXPLICIT_STAGED_CONFIRMATION_REQUIRED')
    accounts = {}
    for role in ('COMPANY', 'DEVELOPER'):
        email, password = required('E2E_' + role + '_EMAIL'), required('E2E_' + role + '_PASSWORD')
        if tag not in email.split('@')[0] or len(email.split('@')) != 2:
            raise Stop('UNTAGGED_TEST_EMAIL_' + role)
        if len(password) < 12:
            raise Stop('TEST_PASSWORD_TOO_SHORT_' + role)
        jar = http.cookiejar.CookieJar()
        accounts[role] = {'email': email, 'password': password,
                          'client': urllib.request.build_opener(NoRedirect(), urllib.request.HTTPCookieProcessor(jar))}
    if accounts['COMPANY']['email'].casefold() == accounts['DEVELOPER']['email'].casefold():
        raise Stop('TEST_ACCOUNTS_MUST_DIFFER')
    manifest_path = Path(os.environ.get('E2E_MANIFEST_FILE', f'/tmp/firstorder-{tag}-{uuid.uuid4().hex[:8]}.json'))
    manifest = {'run_tag': tag, 'target': target, 'started_at': dt.datetime.now(dt.timezone.utc).isoformat(),
                'registration_requested': register, 'public_signup_tested': register,
                'email_delivery_bypassed': bypass_email, 'confirmation_source': confirmation_source or 'existing_confirmed_accounts',
                'email_delivery_tested': token_provided and confirmation_source == 'delivered_email',
                'accounts': {r: {'email': a['email']} for r, a in accounts.items()},
                'created': {'company_ids': [], 'order_ids': [], 'application_ids': []}, 'steps': [], 'status': 'running'}
    fd = os.open(manifest_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    os.close(fd)

    def save():
        # Keep the manifest private even if the caller chose an existing directory.
        os.chmod(manifest_path, 0o600)
        with manifest_path.open('w', encoding='utf-8') as out:
            json.dump(manifest, out, ensure_ascii=False, indent=2)

    def request(role, method, path, data=None, expected=(200,), step=None):
        if not path.startswith('/api/v1/') or '?' in path or '#' in path:
            raise Stop('NONCANONICAL_REQUEST_PATH')
        label = step or method + ' ' + path
        raw = None if data is None else json.dumps(data).encode()
        req = urllib.request.Request(target + path, data=raw, method=method,
                                     headers={'Content-Type': 'application/json', 'Origin': target, 'Accept': 'application/json',
                                              'User-Agent': 'FirstOrder-E2E/1.0'})
        try:
            response = accounts[role]['client'].open(req, timeout=30)
            status, body = response.status, response.read(1024 * 1024)
        except urllib.error.HTTPError as exc:
            status, body = exc.code, exc.read(1024 * 1024)
        except (urllib.error.URLError, TimeoutError, OSError):
            raise Stop('NETWORK_ERROR', label)
        try:
            obj = json.loads(body) if body else {}
        except (ValueError, UnicodeDecodeError):
            obj = {}
        request_id = obj.get('request_id') if isinstance(obj, dict) else None
        event = {'step': label, 'http_status': status}
        if isinstance(request_id, str) and re.fullmatch(r'[0-9a-fA-F-]{36}', request_id):
            event['request_id'] = request_id
        manifest['steps'].append(event)
        save()
        print(json.dumps(event), flush=True)
        if status not in expected:
            error = obj.get('error', {}) if isinstance(obj, dict) else {}
            raise Stop(safe_code(error.get('code')) if isinstance(error, dict) else 'UNEXPECTED_RESPONSE', label, status)
        if not isinstance(obj, dict):
            raise Stop('INVALID_RESPONSE_OBJECT', label, status)
        return obj

    try:
        save()
        if register:
            for role, account in accounts.items():
                body = {'email': account['email'], 'password': account['password'], 'confirm': account['password'],
                        'name': tag + ' ' + role.lower(), 'role': 'company' if role == 'COMPANY' else 'developer'}
                if role == 'COMPANY':
                    body.update(company=tag + ' test company', city='E2E test city')
                request(role, 'POST', '/api/v1/auth/register', body, (201,), 'register_' + role.lower())
                manifest['accounts'][role]['registration_accepted'] = True
                save()
        for role, account in accounts.items():
            token_hash = os.environ.get('E2E_' + role + '_TOKEN_HASH')
            if token_hash:
                request(role, 'POST', '/api/v1/auth/verify', {'token_hash': token_hash, 'type': 'signup'}, step='verify_' + role.lower())
            login = request(role, 'POST', '/api/v1/auth/login', {'email': account['email'], 'password': account['password']}, step='login_' + role.lower())
            profile = request(role, 'GET', '/api/v1/me', step='profile_' + role.lower())
            manifest['accounts'][role]['user_id'] = profile.get('user_id') or login.get('user_id')
            save()
            if role == 'COMPANY':
                matches = [c for c in profile.get('companies', []) if c.get('member_role') == 'owner' and tag in c.get('name', '')]
                if len(matches) != 1:
                    raise Stop('TAGGED_COMPANY_OWNER_NOT_PROVISIONED', 'profile_company')
                company_id = matches[0]['company_id']
                manifest['created']['company_ids'] = [company_id]
                save()
                company = request(role, 'GET', '/api/v1/companies/' + company_id, step='read_onboarded_company')
                manifest['company_visibility'] = company.get('visibility')
                save()
                if (register or bypass_email) and company.get('visibility') != 'hidden':
                    raise Stop('ONBOARDED_COMPANY_NOT_HIDDEN')
        future = (dt.datetime.now(dt.timezone.utc).date() + dt.timedelta(days=30)).isoformat()
        order = request('COMPANY', 'POST', '/api/v1/orders', {'company_id': company_id, 'title': tag + ' test order',
                        'description': tag + ' tagged end-to-end test; not a real commercial order.',
                        'budget_min': 1000, 'budget_max': 2000, 'deadline': future}, (201,), 'create_draft')
        order_id = order['id']
        manifest['created']['order_ids'].append(order_id)
        save()
        if order.get('status') != 'draft':
            raise Stop('ORDER_NOT_DRAFT')
        order = request('COMPANY', 'POST', '/api/v1/orders/' + order_id + '/publish', step='publish_order')
        if order.get('status') != 'open':
            raise Stop('ORDER_NOT_OPEN')
        request('DEVELOPER', 'GET', '/api/v1/orders/' + order_id, step='developer_reads_order')
        application = request('DEVELOPER', 'POST', '/api/v1/orders/' + order_id + '/applications',
                              {'message': tag + ' test proposal for the tagged E2E order only.',
                               'proposed_price': 1500, 'proposed_deadline': future}, (201,), 'submit_application')
        app_id = application['id']
        manifest['created']['application_ids'].append(app_id)
        save()
        if application.get('status') != 'submitted':
            raise Stop('APPLICATION_NOT_SUBMITTED')
        replies = request('COMPANY', 'GET', '/api/v1/orders/' + order_id + '/applications', step='company_reads_applications')
        if not any(a.get('id') == app_id for a in replies.get('items', [])):
            raise Stop('APPLICATION_MISSING_FROM_COMPANY_LIST')
        selected = request('COMPANY', 'POST', '/api/v1/orders/' + order_id + '/select-application', {'application_id': app_id}, step='select_contractor')
        if selected.get('status') != 'contractor_selected' or selected.get('selected_application_id') != app_id:
            raise Stop('ORDER_SELECTION_MISMATCH')
        accepted = request('DEVELOPER', 'GET', '/api/v1/applications/' + app_id, step='developer_reads_acceptance')
        if accepted.get('status') != 'accepted':
            raise Stop('APPLICATION_NOT_ACCEPTED')
        manifest['status'] = 'passed'
        manifest['cleanup_note'] = 'Review IDs and tagged email/name before cleanup; clear selected_application_id before deleting order/application history. Runner performs no deletion.'
        save()
    except Stop as exc:
        manifest['status'] = 'blocked' if exc.status == 401 and exc.step.startswith('login_') else 'failed'
        manifest['failure'] = {'code': exc.code, 'step': exc.step, 'http_status': exc.status}
        save()
        raise
    except Exception:
        manifest['status'] = 'failed'
        manifest['failure'] = {'code': 'UNEXPECTED_RUNNER_ERROR'}
        save()
        raise
    finally:
        print(json.dumps({'manifest': str(manifest_path), 'status': manifest['status']}), flush=True)


if __name__ == '__main__':
    try:
        main()
    except Stop as exc:
        print(json.dumps({'result': 'stopped', 'code': exc.code, 'step': exc.step, 'http_status': exc.status}), flush=True)
        sys.exit(3)
    except Exception:
        # Do not dump request objects or tracebacks: they can contain credentials.
        print(json.dumps({'result': 'stopped', 'code': 'UNEXPECTED_RUNNER_ERROR'}), flush=True)
        sys.exit(2)
