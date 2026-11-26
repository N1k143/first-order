import unittest
import dgis_pages as d

ID = '10000000000000001'
URL = d.BASE + '/astana/firm/' + ID


def page(contacts='', recommendations='', close=True):
    body = ('<html><body><section><header><h1>Пример кафе</h1><span>Кафе</span></header>'
            '<nav>Контакты<a href="/astana/firm/' + ID + '/tab/info">Инфо</a></nav>'
            '<a href="/astana/geo/123456789">Тестовая улица, 10</a>'
            '<a href="tel:+77001234567">Показать телефон</a>' + contacts +
            '<h2>Похожие организации</h2>' + recommendations + '</section>')
    return body + ('</body></html>' if close else '')


class PublicHTMLTests(unittest.TestCase):
    def test_recommendations_do_not_supply_website_or_phone(self):
        ad = '<a href="https://other.example.com">Сайт</a><a href="tel:+77009998877">Телефон</a>'
        result = d.parse_firm(page('<a href="https://instagram.com/example">Instagram</a>', ad), URL)
        self.assertEqual(result['website_listing'], 'not_listed')
        self.assertEqual(result['website_status'], 'needs_review')
        self.assertEqual(result['phones'], ['+77001234567'])
        self.assertEqual(result['website_links'], [])
        self.assertEqual(result['address'], 'Тестовая улица, 10')
        self.assertEqual(len(result['platform_links']), 1)

    def test_own_direct_and_tracking_links(self):
        for href in ['https://business.example.com', 'http://link.2gis.ru/1.2/project/' + ID + '/null?https://business.example.com']:
            with self.subTest(href=href):
                result = d.parse_firm(page('<a href="' + href + '">Сайт</a>'), URL)
                self.assertEqual(result['website_listing'], 'listed')
                self.assertEqual(result['website_links'], ['https://business.example.com'])
                self.assertEqual(result['website_status'], 'needs_review')

    def test_other_advertiser_tracking_is_not_owned(self):
        result = d.parse_firm(page('<a href="http://link.2gis.ru/project/10000000000000002/null?https://other.example.com">Сайт</a>'), URL)
        self.assertEqual(result['website_links'], [])
        self.assertEqual(result['website_listing'], 'not_listed')

    def test_incomplete_or_challenge_page_never_means_no_website(self):
        for raw in [page(close=False), '<html><body><h1>Проверка браузера</h1></body></html>', '<html><body><h1>Пример</h1></body></html>']:
            with self.subTest(raw=raw[:30]):
                result = d.parse_firm(raw, URL)
                self.assertEqual(result['website_listing'], 'unknown')
                self.assertEqual(result['website_status'], 'needs_review')

    def test_unresolved_tracking_and_unsafe_website_are_unknown(self):
        for link in ['http://link.2gis.ru/project/' + ID + '/null?opaque', 'http://127.0.0.1:80/private']:
            result = d.parse_firm(page('<a href="' + link + '">Сайт</a>'), URL)
            self.assertEqual(result['website_listing'], 'unknown')

    def test_search_only_canonical_firms_and_no_script_payloads(self):
        raw = '<html><body><a href="/astana/firm/' + ID + '">Пример</a><a href="/astana/firm/' + ID + '/tab/info">Повтор</a>'
        raw += '<a href="https://evil.example.com/astana/firm/10000000000000002">Чужой</a><script>"/astana/firm/10000000000000003"</script></body></html>'
        self.assertEqual(d.search_links(raw), [{'url': URL, 'name': 'Пример'}])

    def test_urls_and_robots(self):
        for url in ['https://2gis.kz/_/private', 'https://2gis.kz/astana/firm/' + ID + '?login_token=private', 'http://2gis.kz/astana/firm/' + ID, 'https://2gis.kz.evil.example/astana/firm/' + ID]:
            self.assertFalse(d.url_allowed(url))
        self.assertTrue(d.url_allowed(URL))
        rules, delay = d.robot_rules('User-agent: *\nDisallow: /astana/firm/*\nAllow: /astana/firm/' + ID + '$\nCrawl-delay: 4\n')
        client = object.__new__(d.PublicPages)
        client.rules = rules
        self.assertTrue(client.allowed(URL))
        self.assertFalse(client.allowed(d.BASE + '/astana/firm/10000000000000002'))
        self.assertEqual(delay, 4)
        rules, _ = d.robot_rules('User-agent: *\nDisallow: /\nUser-agent: FirstOrder-CompanyDiscovery\nAllow: /astana/\n')
        client.rules = rules
        self.assertTrue(client.allowed(URL))
        with self.assertRaises(d.FetchError):
            d.robot_rules('User-agent: *\nCrawl-delay: 60\n')

    def test_hidden_card_is_not_a_business_card(self):
        self.assertFalse(d.parse_firm('<html><body><div hidden>' + page() + '</div></body></html>', URL)['card_recognized'])
