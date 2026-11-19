package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestImportCSVNormalizesAndOrdersRecords(t *testing.T) {
	input := "\ufeffexternal_id,name,city,email,website_url,website_status\n b ,  Company   B , , HELLO@example.com ,https://example.com,found\na,Компания А,Алматы,,,\n"
	records, err := parseImportCSV(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ExternalID != "a" || records[1].ExternalID != "b" {
		t.Fatalf("records not sorted: %#v", records)
	}
	r := records[1]
	if r.Name != "Company B" || r.City != "Астана" || r.Email != "hello@example.com" {
		t.Fatalf("normalization failed: %#v", r)
	}
	if r.Raw["name"] != "  Company   B " {
		t.Fatal("raw source not preserved")
	}
	if records[0].WebsiteStatus != "needs_review" {
		t.Fatal("wrong default status")
	}
}
func TestImportCSVRejectsInvalidData(t *testing.T) {
	tests := map[string]string{"missing name": "external_id\na\n", "duplicate headers": "external_id,name,name\na,A,A\n", "unexpected header": "external_id,name,password\na,A,secret\n", "duplicate key": "external_id,name\na,A\na,B\n", "invalid UUID": "external_id,name,category_id\na,A,invalid\n", "unsafe website": "external_id,name,website_url\na,A,file:///etc/passwd\n", "unsafe source": "external_id,name,source_url\na,A,javascript:alert(1)\n", "bad status": "external_id,name,website_status\na,A,published\n", "empty name": "external_id,name\na, \n", "invalid email": "external_id,name,email\na,A,not-an-email\n", "wrong column count": "external_id,name\na,A,extra\n"}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseImportCSV(context.Background(), strings.NewReader(input)); err == nil {
				t.Fatal("invalid CSV accepted")
			}
		})
	}
}
func TestImportCSVBoundsAndCancellation(t *testing.T) {
	var csv strings.Builder
	csv.WriteString("external_id,name\n")
	for i := 0; i < importMaxRows; i++ {
		fmt.Fprintf(&csv, "%d,Company\n", i)
	}
	records, err := parseImportCSV(context.Background(), strings.NewReader(csv.String()))
	if err != nil || len(records) != importMaxRows {
		t.Fatalf("limit boundary failed: %v", err)
	}
	csv.WriteString("overflow,Company\n")
	if _, err = parseImportCSV(context.Background(), strings.NewReader(csv.String())); err == nil {
		t.Fatal("row limit not enforced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = parseImportCSV(ctx, strings.NewReader("external_id,name\na,A\n")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
}
func TestImportFailureDoesNotExposeInternalError(t *testing.T) {
	secret := errors.New("password=secret host=private database SQL statement")
	if got := safeImportMessage(secret); strings.Contains(got, "secret") || strings.Contains(got, "SQL") {
		t.Fatal("internal detail exposed")
	}
	if got := safeImportMessage(importFailure("Источник импорта недоступен")); got != "Источник импорта недоступен" {
		t.Fatal("safe error lost")
	}
}
