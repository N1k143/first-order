package api

import (
	"context"
	"encoding/json"
	"ingeneriya/backend/internal/discovery"
	"strings"
)

type CatalogReleaseItem struct {
	CandidateID string   `json:"candidate_id"`
	ExternalID  string   `json:"external_id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Ready       bool     `json:"ready"`
	Reasons     []string `json:"reasons"`
	CompanyID   *string  `json:"company_id,omitempty"`
}
type CatalogRelease struct {
	Items     []CatalogReleaseItem `json:"items"`
	Created   int                  `json:"created"`
	Skipped   int                  `json:"skipped"`
	Published int                  `json:"published"`
}

// PrepareCatalog is an operator-only database operation, never an HTTP endpoint.
// It does not assign a role, create an Auth user, or modify existing companies.
func (s *Server) PrepareCatalog(ctx context.Context, apply, publish bool) (CatalogRelease, error) {
	result := CatalogRelease{Items: []CatalogReleaseItem{}}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer rollbackImport(tx)
	rows, err := tx.Query(ctx, `SELECT id::text FROM importer.company_candidates WHERE source='osm_astana' AND review_status='pending' ORDER BY external_id LIMIT 100 FOR UPDATE`)
	if err != nil {
		return result, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(1730191920)"); err != nil {
		return result, err
	}
	for _, id := range ids {
		row, err := loadCandidate(ctx, tx, id, false)
		if err != nil {
			return result, err
		}
		quality := discovery.AssessCatalog(row.Payload)
		item := CatalogReleaseItem{CandidateID: id, ExternalID: row.Payload.ExternalID, Name: row.Payload.Name, Category: quality.CategoryName, Ready: quality.Ready, Reasons: quality.Reasons}
		// Human drafts are deliberately not auto-approved, even if source quality passes.
		if len(row.Draft) > 0 {
			item.Ready = false
			item.Reasons = append(item.Reasons, "manual_draft_requires_review")
		}
		if item.Ready {
			fields, err := proposedCompany(row)
			if err != nil {
				return result, err
			}
			if len(quality.Phones) > 0 {
				fields["phone"] = strings.Join(quality.Phones, "; ")
			} else {
				fields["phone"] = nil
			}
			fields["email"] = quality.Email
			if err = validateProposed(fields); err != nil {
				return result, err
			}
			var duplicate bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM catalog.companies c WHERE `+duplicateWhere+`)`, duplicateArgs(row, fields)...).Scan(&duplicate); err != nil {
				return result, err
			}
			if duplicate {
				item.Ready = false
				item.Reasons = append(item.Reasons, "possible_duplicate")
			}
			if item.Ready && apply {
				var category string
				if err = tx.QueryRow(ctx, `INSERT INTO catalog.categories(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, quality.CategoryName).Scan(&category); err != nil {
					return result, err
				}
				var company string
				visibility := "hidden"
				if publish {
					visibility = "published"
				}
				if err = tx.QueryRow(ctx, `INSERT INTO catalog.companies(name,category_id,city,address,phone,email,website_url,website_status,website_checked_at,source_url,visibility) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, fields["name"], category, fields["city"], fields["address"], fields["phone"], fields["email"], fields["website_url"], fields["website_status"], row.Payload.Check.CheckedAt, row.Payload.SourceURL, visibility).Scan(&company); err != nil {
					return result, err
				}
				before := row
				draft := map[string]any{"category_id": category, "category_name": nil, "phone": fields["phone"], "email": quality.Email}
				raw, _ := json.Marshal(draft)
				reason := "Операторская подготовка каталога: источник OSM, нормализация контактов и категории. Наличие сайта — по доказательствам; отсутствие сайта не утверждается."
				if _, err = tx.Exec(ctx, `UPDATE importer.company_candidates SET draft=$2,review_status='approved',company_id=$3,reviewed_at=now(),reviewed_by=NULL,review_note=$4 WHERE id=$1`, id, raw, company, reason); err != nil {
					return result, err
				}
				after, err := loadCandidate(ctx, tx, id, false)
				if err != nil {
					return result, err
				}
				b, _ := json.Marshal(before)
				a, _ := json.Marshal(after)
				if _, err = tx.Exec(ctx, `INSERT INTO importer.candidate_actions(candidate_id,moderator_id,actor_type,action,reason,before_state,after_state) VALUES($1,NULL,'operator','approve',$2,$3,$4)`, id, reason, b, a); err != nil {
					return result, err
				}
				if _, err = tx.Exec(ctx, `INSERT INTO catalog.moderation_actions(moderator_id,actor_type,company_id,action,reason) VALUES(NULL,'operator',$1,'approve',$2)`, company, reason); err != nil {
					return result, err
				}
				if publish {
					if _, err = tx.Exec(ctx, `INSERT INTO catalog.moderation_actions(moderator_id,actor_type,company_id,action,reason) VALUES(NULL,'operator',$1,'publish',$2)`, company, reason); err != nil {
						return result, err
					}
					result.Published++
				}
				item.CompanyID = &company
				result.Created++
			}
		}
		if !item.Ready {
			result.Skipped++
		}
		result.Items = append(result.Items, item)
	}
	if apply {
		if err = tx.Commit(ctx); err != nil {
			return result, err
		}
	}
	return result, nil
}
