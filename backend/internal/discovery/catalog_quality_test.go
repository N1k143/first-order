package discovery

import (
	"reflect"
	"testing"
)

func TestCatalogContactsAndEligibility(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want []string
	}{
		{"+7 (7172) 92 24 25; 8-701-945-10-11;+7 701 945 10 11", []string{"+77172922425", "+77019451011"}},
		{"+77786366263 watsap", []string{"+77786366263"}},
		{"160;5757;123456789012;+777712345678", []string{}},
	} {
		if got := CatalogPhones(tc.raw); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("phones for %q: %v", tc.raw, got)
		}
	}
	c := Candidate{Name: "Company", City: "Астана", Category: "shop:florist", SourceURL: "https://www.openstreetmap.org/node/123", Phone: "+77011234567"}
	if result := AssessCatalog(c); !result.Ready || result.CategoryName != "Цветы, сад и сельское хозяйство" {
		t.Fatal("valid company not eligible")
	}
	c.Phone = "160"
	if AssessCatalog(c).Ready {
		t.Fatal("ambiguous shortcode alone accepted")
	}
	c.Email = "business@mail.ru"
	if !AssessCatalog(c).Ready {
		t.Fatal("valid email is a usable business contact, not evidence of a website")
	}
	c.Category = "office:government"
	if AssessCatalog(c).Ready {
		t.Fatal("government published as a company")
	}
	c.Category = "office:research"
	if AssessCatalog(c).Ready {
		t.Fatal("unreviewed category guessed")
	}
}
