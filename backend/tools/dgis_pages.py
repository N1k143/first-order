#!/usr/bin/env python3
"""Bounded 2GIS public-HTML pilot. Never publishes data or bypasses access controls."""
import argparse
import datetime as dt
from html.parser import HTMLParser
import json
import ipaddress
import http.client
import os
from pathlib import Path
import re
import time
import urllib.error
import urllib.parse
import urllib.request

BASE = 'https://2gis.kz'
UA = 'FirstOrder-CompanyDiscovery/1.0 (+https://e1.iq-home.kz)'
MAX_BYTES = 4 * 1024 * 1024
FIRM = re.compile(r'^/astana/firm/([0-9]{8,24})(?:/|$)')
VOID = {'area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input', 'link', 'meta', 'param', 'source', 'track', 'wbr'}
PLATFORMS = {'instagram.com', 'facebook.com', 'vk.com', 't.me', 'telegram.me', 'wa.me', 'whatsapp.com',
             'youtube.com', 'youtu.be', 'tiktok.com', 'ok.ru', 'taplink.cc', 'linktr.ee', 'kaspi.kz',
             '2gis.kz', '2gis.ru', '2gis.com', 'yandex.ru', 'yandex.kz', 'google.com', 'glovoapp.com', 'wolt.com'}
STOP = {'Похожие организации', 'Похожие компании', 'Нашли ошибку в данных?', 'Реклама в 2ГИС'}


def clean(text):
    return ' '.join(text.replace('\u200b', '').replace('\ufeff', '').split())


def url_allowed(url, robots=False):
    u = urllib.parse.urlsplit(url)
    if u.scheme != 'https' or u.netloc != '2gis.kz' or u.fragment or u.query:
        return False
    if robots and u.path == '/robots.txt':
        return True
    return bool(re.fullmatch(r'/astana/firm/[0-9]{8,24}', u.path) or re.fullmatch(r'/astana/search/[^/]+', u.path))


class Node:
    def __init__(self, tag='', attrs=(), parent=None):
        self.tag, self.attrs, self.parent, self.children = tag, dict(attrs), parent, []
        style = self.attrs.get('style', '').lower().replace(' ', '')
        self.hidden = (parent is not None and parent.hidden) or tag in {'script', 'style', 'template'} or 'hidden' in self.attrs or self.attrs.get('aria-hidden') == 'true' or 'display:none' in style

    def walk(self):
        if self.hidden:
            return
        yield self
        for child in self.children:
            if isinstance(child, Node):
                yield from child.walk()

    def text(self):
        if self.hidden:
            return ''
        return clean(' '.join(c if isinstance(c, str) else c.text() for c in self.children))


class Document(HTMLParser):
    def __init__(self, raw):
        super().__init__(convert_charrefs=True)
        if len(raw.encode()) > MAX_BYTES:
            raise ValueError('page_too_large')
        self.root = Node()
        self.stack = [self.root]
        self.count = 0
        self.feed(raw)
        self.complete = len(self.stack) == 1 and bool(re.search(r'</html\s*>\s*$', raw, re.I))

    def handle_starttag(self, tag, attrs):
        self.count += 1
        if self.count > 100000 or len(self.stack) > 256:
            raise ValueError('html_complexity_limit')
        node = Node(tag, attrs, self.stack[-1])
        self.stack[-1].children.append(node)
        if tag not in VOID:
            self.stack.append(node)

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)
        if tag not in VOID:
            self.handle_endtag(tag)

    def handle_endtag(self, tag):
        for index in range(len(self.stack) - 1, 0, -1):
            if self.stack[index].tag == tag:
                self.stack = self.stack[:index]
                return

    def handle_data(self, data):
        if not self.stack[-1].hidden:
            self.stack[-1].children.append(data)


def firm_url(href):
    try:
        u = urllib.parse.urlsplit(urllib.parse.urljoin(BASE, href))
    except ValueError:
        return None
    match = FIRM.match(u.path)
    if u.scheme not in {'http', 'https'} or u.netloc != '2gis.kz' or not match:
        return None
    return BASE + '/astana/firm/' + match[1]


def search_links(raw):
    result, seen = [], set()
    for node in Document(raw).root.walk():
        if node.tag == 'a':
            url = firm_url(node.attrs.get('href', ''))
            if url and url not in seen and node.text():
                result.append({'url': url, 'name': node.text()})
                seen.add(url)
    return result


def platform(host):
    return any(host == domain or host.endswith('.' + domain) for domain in PLATFORMS)


def external_link(href, external_id, depth=0):
    if depth > 3 or len(href) > 8192:
        return None, 'unresolved_tracking_link'
    try:
        u = urllib.parse.urlsplit(href)
    except ValueError:
        return None, 'unsupported_link'
    host = (u.hostname or '').lower().rstrip('.')
    if host in {'link.2gis.ru', 'link.2gis.kz'}:
        # Public HTML advertising links include the owning firm ID. A different
        # advertiser's target must never become this business's website.
        if external_id not in u.path.split('/'):
            return None, 'other_business'
        target = urllib.parse.unquote(u.query)
        if not target.startswith(('http://', 'https://')):
            return None, 'unresolved_tracking_link'
        return external_link(target, external_id, depth + 1)
    if u.scheme not in {'http', 'https'} or not host or u.username or u.password:
        return None, 'unsupported_link'
    try:
        if u.port not in {None, 80, 443}:
            return None, 'unsupported_link'
    except ValueError:
        return None, 'unsupported_link'
    try:
        if not ipaddress.ip_address(host).is_global:
            return None, 'unsupported_link'
    except ValueError:
        if '.' not in host or host.endswith(('.localhost', '.local')):
            return None, 'unsupported_link'
    if platform(host):
        return href, 'platform'
    return urllib.parse.urlunsplit((u.scheme, u.netloc, u.path, u.query, '')), 'website'


def contact_items(root):
    """Yield visible nodes only before recommendations/advertisements begin."""
    stopped = [False]
    def walk(node):
        if node.hidden or stopped[0]:
            return
        if node.text() in STOP:
            stopped[0] = True
            return
        yield node
        for child in node.children:
            if stopped[0]:
                return
            if isinstance(child, str):
                if clean(child) in STOP:
                    stopped[0] = True
                    return
            else:
                yield from walk(child)
    return list(walk(root))


def parse_firm(raw, url):
    if not url_allowed(url) or not FIRM.match(urllib.parse.urlsplit(url).path):
        raise ValueError('invalid_firm_url')
    external_id = FIRM.match(urllib.parse.urlsplit(url).path)[1]
    document = Document(raw)
    result = {'source': '2gis_public_html', 'external_id': external_id, 'source_url': url, 'city': 'Астана',
              'name': None, 'category': None, 'card_recognized': False, 'html_complete': document.complete, 'address': None, 'phones': [], 'emails': [], 'platform_links': [],
              'website_links': [], 'website_listing': 'unknown', 'website_status': 'needs_review',
              'reason': 'incomplete_company_card', 'checked_at': dt.datetime.now(dt.timezone.utc).isoformat()}
    headings = [n for n in document.root.walk() if n.tag == 'h1']
    if len(headings) != 1 or not headings[0].text():
        return result
    result['name'] = headings[0].text()
    root = headings[0].parent
    while root and root.tag not in {'body', 'html', ''}:
        links = [n for n in root.walk() if n.tag == 'a']
        own = any(n.attrs.get('href', '').startswith('/astana/firm/' + external_id + '/tab/') for n in links)
        address = any(n.attrs.get('href', '').startswith('/astana/geo/') for n in links)
        if own and address and 'Контакты' in root.text():
            break
        root = root.parent
    if root is None or root.tag in {'body', 'html', ''}:
        return result
    result['card_recognized'] = True
    nodes = contact_items(root)
    unresolved = False
    for node in nodes:
        if node.tag != 'a':
            continue
        href, text = node.attrs.get('href', ''), node.text()
        if href.startswith('/astana/geo/') and text and result['address'] is None:
            result['address'] = text
        elif href.startswith('tel:'):
            phone = re.sub(r'[^0-9+]', '', urllib.parse.unquote(href[4:]).split('?')[0])
            if re.fullmatch(r'\+?[0-9]{10,15}', phone):
                result['phones'].append(phone)
        elif href.startswith('mailto:'):
            email = urllib.parse.unquote(href[7:]).split('?')[0]
            if re.fullmatch(r'[^\s@]+@[^\s@]+\.[^\s@]+', email):
                result['emails'].append(email)
        elif href.startswith(('http://', 'https://')):
            link, kind = external_link(href, external_id)
            if kind == 'website':
                result['website_links'].append(link)
            elif kind == 'platform' and link and '2gis.' not in urllib.parse.urlsplit(link).netloc:
                result['platform_links'].append(link)
            elif kind in {'unresolved_tracking_link', 'unsupported_link'}:
                unresolved = True
    for key in ['phones', 'emails', 'platform_links', 'website_links']:
        result[key] = list(dict.fromkeys(result[key]))
    heading_parent = headings[0].parent
    for node in heading_parent.walk():
        if node.tag == 'span' and node.text() and node.text() != result['name'] and len(node.text()) <= 100:
            if node.text() in {'Кафе', 'Ресторан', 'Автосервис', 'Салон красоты', 'Магазин'}:
                result['category'] = node.text()
                break
    if result['website_links']:
        result.update(website_listing='listed', reason='website_link_in_contacts')
    elif document.complete and not unresolved and result['address'] and any(result[k] for k in ['phones', 'emails', 'platform_links']):
        result.update(website_listing='not_listed', reason='website_not_listed_in_2gis')
    return result


class FetchError(Exception):
    pass


class NoRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, response, code, message, headers, target):
        raise FetchError('redirect_stopped')


def robot_rules(raw):
    groups, agents, directives = [], [], []
    for line in raw.splitlines() + ['User-agent: __end__']:
        key, separator, value = line.split('#', 1)[0].partition(':')
        key, value = key.strip().lower(), value.strip()
        if not separator:
            continue
        if key == 'user-agent':
            if directives:
                groups.append((agents, directives))
                agents, directives = [], []
            agents.append(value.lower())
        elif key in {'allow', 'disallow', 'crawl-delay'} and value:
            directives.append((key, value))
    if not groups:
        raise FetchError('invalid_robots')
    product = UA.split('/')[0].lower()
    applicable = []
    for agents, directives in groups:
        scores = [0 if a == '*' else len(a) for a in agents if a == '*' or a in product]
        if scores:
            applicable.append((max(scores), directives))
    best = max((score for score, _ in applicable), default=-1)
    rules, delay = [], 3.0
    for score, directives in applicable:
        if score != best:
            continue
        for key, value in directives:
            if key == 'crawl-delay':
                try:
                    seconds = float(value)
                except ValueError:
                    raise FetchError('invalid_robots_delay') from None
                if not 0 <= seconds <= 30:
                    raise FetchError('robots_delay_requires_manual_review')
                delay = max(delay, seconds)
            else:
                rules.append((key, value))
    return rules, delay


class PublicPages:
    def __init__(self):
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirects())
        self.last = 0
        self.robots = self._get(BASE + '/robots.txt', robots=True)
        self.rules, self.delay = robot_rules(self.robots)

    def allowed(self, url):
        path = urllib.parse.urlsplit(url).path
        matches = []
        for kind, pattern in self.rules:
            end = pattern.endswith('$')
            expression = '^' + re.escape(pattern[:-1] if end else pattern).replace(r'\*', '.*') + ('$' if end else '')
            if re.search(expression, path):
                matches.append((len(pattern.replace('*', '')), kind == 'allow'))
        return max(matches, default=(0, True))[1]

    def _get(self, url, robots=False):
        if not url_allowed(url, robots):
            raise FetchError('url_not_allowed')
        time.sleep(max(0, getattr(self, 'delay', 3) - (time.monotonic() - self.last)))
        self.last = time.monotonic()
        request = urllib.request.Request(url, headers={'User-Agent': UA, 'Accept': 'text/plain' if robots else 'text/html', 'Accept-Language': 'ru-RU,ru;q=0.9'})
        try:
            with self.opener.open(request, timeout=25) as response:
                if response.status != 200:
                    raise FetchError('http_' + str(response.status))
                content = response.headers.get('Content-Type', '')
                if ('text/plain' if robots else 'text/html') not in content:
                    raise FetchError('unexpected_content_type')
                raw = response.read(MAX_BYTES + 1)
                if len(raw) > MAX_BYTES:
                    raise FetchError('response_too_large')
                return raw.decode('utf-8', errors='replace')
        except urllib.error.HTTPError as error:
            raise FetchError('http_' + str(error.code)) from None
        except (urllib.error.URLError, OSError, http.client.HTTPException):
            raise FetchError('source_unavailable') from None

    def get(self, url):
        if not self.allowed(url):
            raise FetchError('robots_disallowed')
        return self._get(url)


def run():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--input', type=Path, help='Saved public HTML, processed without network requests')
    mode.add_argument('--fetch', action='store_true', help='Explicit bounded public-page pilot; review 2GIS terms before use')
    parser.add_argument('--url', help='Canonical firm URL for --input; omit to parse a saved search page')
    parser.add_argument('--query', default='Кафе')
    parser.add_argument('--limit', type=int, default=3, choices=range(1, 11), metavar='1..10')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists():
        parser.error('output already exists')
    if not 1 <= len(args.query.strip()) <= 100 or '/' in args.query:
        parser.error('query must contain 1–100 characters without slash')
    if args.url and not args.input:
        parser.error('--url is only used with --input')
    records, links, error = [], [], None
    try:
        if args.input:
            if args.input.stat().st_size > MAX_BYTES:
                raise ValueError('input_too_large')
            raw = args.input.read_text(encoding='utf-8')
            if args.url:
                records.append(parse_firm(raw, args.url))
            else:
                links = search_links(raw)[:args.limit]
        else:
            pages = PublicPages()
            url = BASE + '/astana/search/' + urllib.parse.quote(args.query.strip(), safe='')
            links = search_links(pages.get(url))[:args.limit]
            if not links:
                raise FetchError('no_public_cards_or_challenge')
            for link in links:
                record = parse_firm(pages.get(link['url']), link['url'])
                records.append(record)
                if not record['card_recognized']:
                    raise FetchError('incomplete_page_or_challenge')
    except (FetchError, ValueError, OSError) as failure:
        error = str(failure) if isinstance(failure, (FetchError, ValueError)) else 'input_unavailable'
    counts = {key: sum(r['website_listing'] == key for r in records) for key in ['listed', 'not_listed', 'unknown']}
    report = {'source': '2gis_public_html', 'city': 'Астана', 'pilot_only': True, 'error': error,
              'website_listing_counts': counts, 'candidates': records, 'discovered_links': links,
              'notice': 'No website link in 2GIS does not establish that the business has no website. No automatic publication.'}
    descriptor = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, 'w', encoding='utf-8') as handle:
        json.dump(report, handle, ensure_ascii=False, indent=2)
        handle.write('\n')
    print(json.dumps({'checked': len(records), **counts, 'error': error}))
    return 1 if error else 0


if __name__ == '__main__':
    os.umask(0o077)
    raise SystemExit(run())
