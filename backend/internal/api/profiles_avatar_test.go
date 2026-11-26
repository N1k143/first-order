package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"
)

func avatarTestImage(t *testing.T, format string, width int) string {
	t.Helper()
	var raw bytes.Buffer
	pixels := image.NewRGBA(image.Rect(0, 0, width, 1))
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&raw, pixels, nil)
	} else {
		err = png.Encode(&raw, pixels)
	}
	if err != nil {
		t.Fatal(err)
	}
	return "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(raw.Bytes())
}
func TestAvatarValidation(t *testing.T) {
	pngImage := avatarTestImage(t, "png", 2)
	for _, tc := range []struct {
		name, value string
		valid       bool
	}{
		{"https", "https://images.example.test/avatar.png", true},
		{"http", "http://images.example.test/avatar.jpg", true},
		{"PNG", pngImage, true},
		{"JPEG", avatarTestImage(t, "jpeg", 2), true},
		{"WebP", "data:image/webp;base64,UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA", true},
		{"empty", "", false},
		{"script URL", "javascript:alert(1)", false},
		{"credentials URL", "https://user:password@images.example.test/a", false},
		{"long URL", "https://images.example.test/" + strings.Repeat("a", 2048), false},
		{"SVG data", "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte("<svg/>")), false},
		{"claimed JPEG with PNG bytes", strings.Replace(pngImage, "image/png", "image/jpeg", 1), false},
		{"corrupt data", "data:image/png;base64,bm90LWFuLWltYWdl", false},
		{"invalid base64", "data:image/png;base64,!", false},
		{"oversize", "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, maxAvatarBytes+1)), false},
		{"oversize pixels", avatarTestImage(t, "png", 2049), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := profilePatch{AvatarURL: &tc.value}
			if err := p.validate(); (err == nil) != tc.valid {
				t.Fatalf("valid=%v; error=%v", tc.valid, err)
			}
		})
	}
	var p profilePatch
	req := httptest.NewRequest("PATCH", "/me/profile", strings.NewReader(`{"avatar_url":null}`))
	if err := readJSON(req, &p); err != nil {
		t.Fatal(err)
	}
	if err := p.validate(); err != nil || !p.present["avatar_url"] || p.AvatarURL != nil {
		t.Fatal("explicit avatar null must clear only avatar", err)
	}
	for _, body := range []string{`{"avatar_url":null,"role":"moderator"}`, `{"avatar_url":null,"user_id":"someone"}`, `{"avatar_url":42}`} {
		var p profilePatch
		if err := json.Unmarshal([]byte(body), &p); err == nil {
			t.Fatal("avatar update accepted an invalid or privileged field")
		}
	}
}
func TestIntegrationAvatarPersistenceAndRemoval(t *testing.T) {
	s, pool, f := dbTest(t)
	avatar := avatarTestImage(t, "png", 2)
	testStatus(t, testRequest(t, s, "PATCH", "/me/profile", f.Developer, map[string]any{"avatarUrl": avatar}), 200)
	me := testRequest(t, s, "GET", "/me", f.Developer, nil)
	testStatus(t, me, 200)
	if testObject(t, me)["avatarUrl"] != avatar {
		t.Fatal("compat profile omitted persisted avatar")
	}
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/me/profile", f.Developer, map[string]any{"bio": "keep avatar"}), 200)
	profile := testRequest(t, s, "GET", "/api/v1/profiles/"+f.Developer, f.OtherDeveloper, nil)
	testStatus(t, profile, 200)
	if testObject(t, profile)["avatar_url"] != avatar {
		t.Fatal("omitted avatar changed or public profile omitted it")
	}
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/me/profile", f.Developer, map[string]any{"avatar_url": "data:image/png;base64,bm90LWltYWdl"}), 422)
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/me/profile", f.Developer, map[string]any{"avatar_url": nil, "role": "moderator"}), 400)
	var stored, role string
	if err := pool.QueryRow(context.Background(), "SELECT avatar_url,role FROM profiles.profiles WHERE user_id=$1", f.Developer).Scan(&stored, &role); err != nil || stored != avatar || role != "developer" {
		t.Fatal("rejected request changed profile", err)
	}
	testStatus(t, testRequest(t, s, "PATCH", "/me/profile", f.Developer, map[string]any{"avatarUrl": nil}), 200)
	me = testRequest(t, s, "GET", "/me", f.Developer, nil)
	testStatus(t, me, 200)
	fields := testObject(t, me)
	if value, ok := fields["avatarUrl"]; !ok || value != nil || fields["bio"] != "keep avatar" {
		t.Fatal("avatar removal did not persist or changed another field")
	}
}
