package api

import (
	"ingeneriya/backend/internal/discovery"
	"net/http"
	"strings"
)

// Only public business provenance is included; raw tags, drafts and audit identities stay private.
const catalogSourcesSQL = `coalesce((SELECT jsonb_agg(jsonb_build_object('source',cc.source,'source_url',cc.payload->>'source_url','attribution',cc.payload->>'attribution','license','ODbL-1.0','license_url','https://www.openstreetmap.org/copyright','reviewed_at',cc.reviewed_at) ORDER BY cc.id) FROM importer.company_candidates cc WHERE cc.company_id=c.id AND cc.review_status='approved'),'[]'::jsonb) AS sources`

func catalogAttribution(w http.ResponseWriter) {
	w.Header().Set("Link", `<https://www.openstreetmap.org/copyright>; rel="license"`)
	w.Header().Set("X-Data-Attribution", "OpenStreetMap contributors; ODbL-1.0")
}
func (s *Server) catalogFilters(w http.ResponseWriter, r *http.Request) error {
	if s.DB == nil {
		return dbUnavailable()
	}
	// Facets only count published records; hidden company names/categories are never leaked.
	cities, err := listJSON(r.Context(), s.DB, `SELECT city,count(*) AS total FROM catalog.companies WHERE visibility='published' GROUP BY city ORDER BY city`)
	if err != nil {
		return err
	}
	categories, err := listJSON(r.Context(), s.DB, `SELECT cat.id,cat.name,count(*) AS total FROM catalog.categories cat JOIN catalog.companies c ON c.category_id=cat.id WHERE c.visibility='published' GROUP BY cat.id,cat.name ORDER BY cat.name,cat.id`)
	if err != nil {
		return err
	}
	statuses, err := listJSON(r.Context(), s.DB, `SELECT website_status,count(*) AS total FROM catalog.companies WHERE visibility='published' GROUP BY website_status ORDER BY website_status`)
	if err != nil {
		return err
	}
	var hasOSM bool
	if err = s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM importer.company_candidates cc JOIN catalog.companies c ON c.id=cc.company_id WHERE cc.source='osm_astana' AND cc.review_status='approved' AND c.visibility='published')`).Scan(&hasOSM); err != nil {
		return err
	}
	var attribution any
	var license any
	if hasOSM {
		catalogAttribution(w)
		attribution = discovery.Attribution
		license = "https://www.openstreetmap.org/copyright"
	}
	return respond(w, 200, map[string]any{"cities": cities, "categories": categories, "website_statuses": statuses, "attribution": attribution, "license_url": license})
}
func decorateCatalogCompanies(w http.ResponseWriter, items []map[string]any) {
	// Field is derived from the stored legacy phone string; existing consumers retain that field.
	for _, item := range items {
		phone, _ := item["phone"].(string)
		item["phones"] = discovery.CatalogPhones(phone)
		email, _ := item["email"].(string)
		website, _ := item["website_url"].(string)
		item["has_contacts"] = len(item["phones"].([]string)) > 0 || strings.TrimSpace(email) != "" || strings.TrimSpace(website) != ""
		if sources, ok := item["sources"].([]any); ok && len(sources) > 0 {
			catalogAttribution(w)
		}
	}
}
