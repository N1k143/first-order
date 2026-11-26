package api

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationCatalogTrigramSearch(t *testing.T) {
	s, p, f := dbTest(t)
	ctx := context.Background()
	extra := newID()
	t.Cleanup(func() { p.Exec(ctx, `DELETE FROM catalog.companies WHERE id=$1`, extra) })
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE catalog.categories SET name=$2 WHERE id=$1`, []any{f.Category, "Кофейни " + f.Category}},
		{`UPDATE catalog.companies SET name='Ёлочка',city='Астана',address='проспект Абая, 10',phone='+7 (777) 123-45-67',website_status='needs_review' WHERE id=$1`, []any{f.Company}},
		{`UPDATE catalog.companies SET name='Ёлочка',city='Астана' WHERE id=$1`, []any{f.HiddenCompany}},
		{`INSERT INTO catalog.companies(id,name,city,address,category_id,website_status) VALUES($1,'Елочко','Астана','Улица другая','` + f.Category + `','found')`, []any{extra}},
	} {
		if _, err := p.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	request := func(q, filters, actor string) map[string]any {
		t.Helper()
		rec := testRequest(t, s, "GET", "/api/v1/companies?category_id="+f.Category+"&q="+url.QueryEscape(q)+filters, actor, nil)
		testStatus(t, rec, 200)
		return testObject(t, rec)
	}
	for _, c := range []struct {
		q, filters string
		total      int
	}{
		{"елочка", "", 2}, {"ЕЛОЧКА", "", 2}, {"ёлчка", "", 1},
		{"кофени", "", 2}, {"елочка абая", "", 1}, {"АБАЯ ёлочка", "", 1},
		{"елочка", "&city=астана&website_status=needs_review", 1},
		{"елочка", "&city=Алматы", 0}, {"елочка", "&website_status=found", 1},
		{"+77771234567", "", 1}, {"ел", "", 2}, {"%_", "", 0},
		{"елочка заведомонесуществующееслово", "", 0},
	} {
		t.Run(c.q+c.filters, func(t *testing.T) {
			obj := request(c.q, c.filters, "")
			if obj["total"] != float64(c.total) {
				t.Fatalf("want %d: %#v", c.total, obj)
			}
		})
	}
	first := request("елочка", "&limit=1", "")
	if first["items"].([]any)[0].(map[string]any)["id"] != f.Company {
		t.Fatal("exact name must rank ahead of typo")
	}
	second := request("елочка", "&limit=1&page=2", "")
	if second["total"] != float64(2) || second["items"].([]any)[0].(map[string]any)["id"] != extra {
		t.Fatal("search count/pagination disagree")
	}
	if request("елочка", "&sort=name", "")["total"] != float64(2) {
		t.Fatal("explicit sorting broke search")
	}
	if request("елочка", "&visibility=hidden", f.Moderator)["total"] != float64(1) {
		t.Fatal("moderator hidden search broken")
	}
	rec := testRequest(t, s, "GET", "/api/v1/companies?q="+url.QueryEscape(strings.Repeat("слово ", 13)+"разные"), "", nil)
	// Repeated words are deduplicated; distinct-word overflow is separately bounded.
	testStatus(t, rec, 200)
	rec = testRequest(t, s, "GET", "/api/v1/companies?q="+url.QueryEscape("один два три четыре пять шесть семь восемь девять десять одиннадцать двенадцать тринадцать"), "", nil)
	testStatus(t, rec, 422)
	// Runtime role must have access to the extension schema and helpers.
	cfg := p.Config().Copy()
	cfg.ConnConfig.RuntimeParams["role"] = "firstorder_backend"
	runtime, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	testStatus(t, testRequest(t, New(runtime, s.Config), "GET", "/api/v1/companies?q="+url.QueryEscape("ёлчка")+"&category_id="+f.Category, "", nil), 200)
}

func TestIntegrationCatalogSearchIndexesAndLocalThreshold(t *testing.T) {
	_, p, _ := dbTest(t)
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL enable_seqscan=off; SET LOCAL pg_trgm.word_similarity_threshold=0.4`); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct{ sql, index string }{
		{`SELECT c.id FROM catalog.companies c WHERE catalog.search_word_similar(` + catalogSearchDocument + `,'елочка')`, "companies_search_trgm"},
		{`SELECT id FROM catalog.categories WHERE catalog.search_word_similar(catalog.search_normalize(name),'кофени')`, "categories_search_trgm"},
	} {
		var plan []byte
		if err = tx.QueryRow(ctx, `EXPLAIN(FORMAT JSON) `+check.sql).Scan(&plan); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(plan), check.index) {
			t.Fatalf("indexed matching was not inlined: %s", plan)
		}
	}
	args := []any{}
	// With just two fixtures the planner prefers scanning the primary key in
	// union order. Force bitmap plans to verify eligibility, not a speed claim.
	if _, err = tx.Exec(ctx, `SET LOCAL enable_indexscan=off; SET LOCAL enable_indexonlyscan=off`); err != nil {
		t.Fatal(err)
	}
	clause, _, err := catalogSearchSQL("елочка", &args)
	if err != nil {
		t.Fatal(err)
	}
	var fullPlan []byte
	if err = tx.QueryRow(ctx, `EXPLAIN(FORMAT JSON) SELECT c.id FROM catalog.companies c WHERE c.visibility='published' AND `+clause, args...).Scan(&fullPlan); err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"companies_search_trgm", "categories_search_trgm"} {
		if !strings.Contains(string(fullPlan), index) {
			t.Fatalf("actual search cannot use %s: %s", index, fullPlan)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// A pooled connection must not retain a request's fuzzy threshold.
	var threshold string
	if err = p.QueryRow(ctx, `SHOW pg_trgm.word_similarity_threshold`).Scan(&threshold); err != nil || threshold != "0.6" {
		t.Fatalf("request setting leaked: %s %v", threshold, err)
	}
}
