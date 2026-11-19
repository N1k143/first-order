package api

import (
	"encoding/json"
	"testing"
	"time"
)

func catalogTestRaw(t *testing.T, body string) map[string]json.RawMessage {
	t.Helper()
	var value map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestCompanyPatchPreservesOmissionsAndClearsNullableValues(t *testing.T) {
	fields, err := catalogCompanyFields(catalogTestRaw(t, `{"phone":null,"name":"  Новое название  "}`), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("patch must not synthesize defaults or other fields: %#v", fields)
	}
	phone, present := fields["phone"]
	if !present || phone != nil {
		t.Fatalf("explicit null must clear phone: %#v", fields)
	}
	if fields["name"] != "Новое название" {
		t.Fatalf("name not normalized: %#v", fields)
	}
	if _, present = fields["city"]; present {
		t.Fatal("omitted city would overwrite existing value")
	}
	if _, present = fields["website_status"]; present {
		t.Fatal("omitted status would overwrite existing value")
	}
}

func TestCompanyPayloadCannotChangeProtectedFields(t *testing.T) {
	for _, field := range []string{"id", "created_at", "updated_at", "visibility", "role", "user_id", "actor_id", "is_moderator", "source_record_id", "name = NULL; DROP TABLE catalog.companies;--"} {
		t.Run(field, func(t *testing.T) {
			raw := map[string]json.RawMessage{field: json.RawMessage(`"hidden"`)}
			if _, err := catalogCompanyFields(raw, false); err == nil {
				t.Fatalf("protected/unknown field %q accepted", field)
			}
		})
	}
}

func TestCompanyCreateAndPatchValidation(t *testing.T) {
	for _, body := range []string{`{}`, `{"name":null}`, `{"name":"  "}`, `{"name":42}`, `{"city":null}`, `{"website_status":"pending"}`, `{"category_id":"not-a-uuid"}`, `{"website_checked_at":"2026-11-19"}`, `{"email":"Person <name@example.com>"}`, `{"website_url":"javascript:alert(1)"}`} {
		t.Run(body, func(t *testing.T) {
			if _, err := catalogCompanyFields(catalogTestRaw(t, body), false); err == nil {
				t.Fatalf("invalid patch accepted: %s", body)
			}
		})
	}
	if _, err := catalogCompanyFields(catalogTestRaw(t, `{"city":"Алматы"}`), true); err == nil {
		t.Fatal("create without name accepted")
	}
	fields, err := catalogCompanyFields(catalogTestRaw(t, `{"name":"Компания"}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if fields["city"] != "Астана" || fields["website_status"] != "needs_review" {
		t.Fatalf("create defaults differ from schema: %#v", fields)
	}
	if _, ok := fields["visibility"]; ok {
		t.Fatal("visibility must be managed by separate protected action/default")
	}
}

func TestContactPatchNullAndTimeSemantics(t *testing.T) {
	fields, err := catalogContactFields(catalogTestRaw(t, `{"notes":null,"contacted_at":"2026-11-19T15:00:00+05:00"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["status"]; ok {
		t.Fatal("missing status must preserve the existing status")
	}
	if val, ok := fields["notes"]; !ok || val != nil {
		t.Fatal("explicit null notes must clear them")
	}
	want := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	if got, ok := fields["contacted_at"].(time.Time); !ok || !got.Equal(want) {
		t.Fatalf("contact time not preserved in UTC: %v", fields["contacted_at"])
	}
	for _, body := range []string{`{"status":null}`, `{"status":"accepted"}`, `{"developer_id":"00000000-0000-0000-0000-000000000001"}`, `{"contacted_at":"2026-11-19"}`, `{"notes":{}}`} {
		if _, err = catalogContactFields(catalogTestRaw(t, body)); err == nil {
			t.Errorf("invalid/privileged contact field accepted: %s", body)
		}
	}
}

func TestCorrectionResolutionNeverInterpretsFreeText(t *testing.T) {
	note := "phone=attacker; visibility=published"
	fields, err := catalogResolutionFields(catalogResolution{Status: "approved", ResolutionNote: &note})
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 0 {
		t.Fatalf("resolution note must not generate company mutations: %#v", fields)
	}
	fields, err = catalogResolutionFields(catalogResolution{Status: "approved", Changes: json.RawMessage(`{"phone":null}`)})
	if err != nil {
		t.Fatal(err)
	}
	if val, ok := fields["phone"]; !ok || val != nil {
		t.Fatal("approved structured changes lost explicit null")
	}
	for _, body := range []catalogResolution{
		{Status: "rejected", Changes: json.RawMessage(`{"phone":"+77001234567"}`)},
		{Status: "pending"},
		{Status: "approved", Changes: json.RawMessage(`{"visibility":"published"}`)},
		{Status: "approved", Changes: json.RawMessage(`null`)},
		{Status: "approved", Changes: json.RawMessage(`[]`)},
	} {
		if _, err = catalogResolutionFields(body); err == nil {
			t.Errorf("invalid resolution accepted: %+v", body)
		}
	}
}

func TestCatalogSortDoesNotAcceptSQLExpressions(t *testing.T) {
	for _, value := range []string{"name; DELETE FROM catalog.companies", "random()", "name DESC NULLS FIRST", "c.id"} {
		if _, err := catalogCompanyOrder(value); err == nil {
			t.Errorf("untrusted SQL sort %q accepted", value)
		}
	}
	for _, value := range []string{"", "updated", "name", "name_desc", "newest", "oldest"} {
		if _, err := catalogCompanyOrder(value); err != nil {
			t.Errorf("valid sort %q rejected: %v", value, err)
		}
	}
}

func TestModerationDateBoundsIncludeWholeDate(t *testing.T) {
	from, err := catalogDateFilter("date_from", "2026-11-19", false)
	if err != nil {
		t.Fatal(err)
	}
	to, err := catalogDateFilter("date_to", "2026-11-19", true)
	if err != nil {
		t.Fatal(err)
	}
	if to.Sub(from) != 24*time.Hour-time.Nanosecond {
		t.Fatalf("date end excludes part of day: %v to %v", from, to)
	}
	if _, err = catalogDateFilter("date_from", "2026-02-30", false); err == nil {
		t.Fatal("invalid calendar date accepted")
	}
}
