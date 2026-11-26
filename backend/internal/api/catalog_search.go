package api

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Keep this expression identical to the indexed expression in migration 010.
const catalogSearchDocument = `catalog.search_normalize(coalesce(c.name,'')||' '||coalesce(c.address,'')||' '||coalesce(c.city,'')||' '||coalesce(c.phone,'')||' '||coalesce(c.email,'')||' '||regexp_replace(coalesce(c.phone,''),'[^0-9]','','g'))`

func catalogSearchTerms(query string) ([]string, error) {
	if utf8.RuneCountInString(query) > 240 {
		return nil, invalid("q", "Не более 240 символов")
	}
	query = strings.Map(func(r rune) rune {
		if r == 'ё' || r == 'Ё' {
			return 'е'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, query)
	terms := []string{}
	seen := map[string]bool{}
	for _, term := range strings.Fields(query) {
		if !seen[term] {
			terms = append(terms, term)
			seen[term] = true
		}
	}
	if len(terms) > 12 {
		return nil, invalid("q", "Укажите не более 12 поисковых слов")
	}
	return terms, nil
}

func catalogSearchSQL(query string, args *[]any) (string, string, error) {
	terms, err := catalogSearchTerms(query)
	if err != nil {
		return "", "", err
	}
	if len(terms) == 0 {
		return "FALSE", "", nil
	}
	clauses := []string{}
	placeholders := []string{}
	for _, term := range terms {
		*args = append(*args, term)
		n := len(*args)
		placeholders = append(placeholders, fmt.Sprintf("$%d::text", n))
		// All terms are required, but can match different fields or the category.
		// Short fragments and numeric contacts stay literal, avoiding fuzzy noise.
		match := func(document string) string {
			literal := fmt.Sprintf(`%s LIKE '%%'||$%d::text||'%%'`, document, n)
			if utf8.RuneCountInString(term) < 4 || strings.IndexFunc(term, unicode.IsLetter) < 0 {
				return literal
			}
			if utf8.RuneCountInString(term) == 4 {
				return fmt.Sprintf(`(%s OR (catalog.search_word_similar(%s,$%d) AND catalog.search_similarity($%d,%s)>=0.5))`, literal, document, n, n, document)
			}
			return fmt.Sprintf(`(%s OR catalog.search_word_similar(%s,$%d))`, literal, document, n)
		}
		// Build the matching ID set from two indexed branches. A correlated
		// OR EXISTS would otherwise force scanning companies for category hits.
		document := strings.ReplaceAll(catalogSearchDocument, "c.", "search_company.")
		clauses = append(clauses, `c.id IN (
 SELECT search_company.id FROM catalog.companies search_company WHERE `+match(document)+`
 UNION
 SELECT category_company.id FROM catalog.companies category_company JOIN catalog.categories search_category ON search_category.id=category_company.category_id
 WHERE `+match(`catalog.search_normalize(search_category.name)`)+`)`)
	}
	fullQuery := "concat_ws(' '," + strings.Join(placeholders, ",") + ")"
	ranking := fmt.Sprintf(`(CASE WHEN catalog.search_normalize(c.name)=%[1]s THEN 100 WHEN catalog.search_normalize(c.name) LIKE %[1]s||'%%' THEN 50 WHEN catalog.search_normalize(c.name) LIKE '%%'||%[1]s||'%%' THEN 20 ELSE 0 END
 +3*catalog.search_similarity(%[1]s,catalog.search_normalize(c.name))
 +2*catalog.search_similarity(%[1]s,catalog.search_normalize(coalesce(cat.name,'')))
 +catalog.search_similarity(%[1]s,%[2]s)) DESC,`, fullQuery, catalogSearchDocument)
	return strings.Join(clauses, " AND "), ranking, nil
}
