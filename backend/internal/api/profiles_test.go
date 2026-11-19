package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func profileTestString(s string) *string { return &s }
func TestProfilePatchRejectsPrivilegeFields(t *testing.T) {
	for _, body := range []string{`{"name":"A","role":"moderator"}`, `{"user_id":"00000000-0000-4000-8000-000000000001"}`, `{"is_moderator":true}`} {
		t.Run(body, func(t *testing.T) {
			var patch profilePatch
			r := httptest.NewRequest("PATCH", "/api/v1/me/profile", strings.NewReader(body))
			if err := readJSON(r, &patch); err == nil {
				t.Fatal("privileged field must not be accepted")
			}
		})
	}
}
func TestProfilePatchValidation(t *testing.T) {
	tests := []struct {
		name  string
		patch profilePatch
		ok    bool
	}{
		{"empty", profilePatch{}, false}, {"blank name", profilePatch{Name: profileTestString(" \t ")}, false}, {"long unicode name", profilePatch{Name: profileTestString(strings.Repeat("Я", 201))}, false}, {"unicode boundary", profilePatch{Name: profileTestString(strings.Repeat("Я", 200))}, true}, {"clear bio", profilePatch{Bio: profileTestString("")}, true}, {"unsafe URL", profilePatch{GithubURL: profileTestString("javascript:alert(1)")}, false}, {"https URL", profilePatch{GithubURL: profileTestString("https://github.com/example")}, true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.patch.validate(); (err == nil) != tt.ok {
				t.Fatalf("validate=%v; want ok=%v", err, tt.ok)
			}
		})
	}
}
func TestPortfolioValidation(t *testing.T) {
	if err := (&portfolioInput{}).validate(true); err == nil {
		t.Fatal("create requires title")
	}
	if err := (&portfolioInput{Title: profileTestString(" ")}).validate(true); err == nil {
		t.Fatal("blank title accepted")
	}
	if err := (&portfolioInput{Description: profileTestString("updated")}).validate(false); err != nil {
		t.Fatal(err)
	}
	if err := (&portfolioInput{Title: profileTestString("Work"), ProjectURL: profileTestString("file:///etc/passwd")}).validate(true); err == nil {
		t.Fatal("unsafe project URL accepted")
	}
}
func TestMemberInputDoesNotPermitPlatformRoles(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	for _, role := range []string{"developer", "moderator", "system_importer"} {
		if err := (&memberInput{UserID: id, MemberRole: role}).validate(); err == nil {
			t.Fatalf("accepted platform role %s as membership", role)
		}
	}
	body := memberInput{UserID: id}
	if err := body.validate(); err != nil {
		t.Fatal(err)
	}
	if body.MemberRole != "representative" {
		t.Fatal("unexpected default role")
	}
	if err := (&memberInput{UserID: "not-a-uuid", MemberRole: "owner"}).validate(); err == nil {
		t.Fatal("invalid user id accepted")
	}
}

func TestProfileNullableFieldsDistinguishOmission(t *testing.T) {
	var patch profilePatch
	req := httptest.NewRequest("PATCH", "/me/profile", strings.NewReader(`{"bio":null,"github_url":null}`))
	if err := readJSON(req, &patch); err != nil {
		t.Fatal(err)
	}
	if err := patch.validate(); err != nil {
		t.Fatal(err)
	}
	if !profileHas(patch.present, "bio", patch.Bio) || patch.Bio != nil {
		t.Fatal("explicit null bio not retained")
	}
	if profileHas(patch.present, "name", patch.Name) || profileHas(patch.present, "skills", patch.Skills) {
		t.Fatal("omitted fields unexpectedly present")
	}
	for _, body := range []string{`{"name":null}`, `{"bio":42}`, `{"updated_at":null}`, `null`} {
		var p profilePatch
		r := httptest.NewRequest("PATCH", "/me/profile", strings.NewReader(body))
		err := readJSON(r, &p)
		if err == nil {
			err = p.validate()
		}
		if err == nil {
			t.Errorf("invalid patch accepted: %s", body)
		}
	}
	var item portfolioInput
	req = httptest.NewRequest("PATCH", "/me/portfolio/id", strings.NewReader(`{"description":null,"project_url":null}`))
	if err := readJSON(req, &item); err != nil {
		t.Fatal(err)
	}
	if err := item.validate(false); err != nil {
		t.Fatal(err)
	}
	if !profileHas(item.present, "description", item.Description) || profileHas(item.present, "title", item.Title) {
		t.Fatal("portfolio null/omission confused")
	}
}
