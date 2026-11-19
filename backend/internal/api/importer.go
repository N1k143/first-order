package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	importMaxRows  = 10000
	importMaxBytes = 20 << 20
)

func (s *Server) registerImports(r chi.Router) {
	r.MethodFunc("POST", "/imports", handle(s.createImport))
	r.MethodFunc("GET", "/imports", handle(s.listImports))
	r.MethodFunc("GET", "/imports/{run_id}", handle(s.getImport))
}
func (s *Server) createImport(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator", "system_importer"); err != nil {
		return err
	}
	var body struct {
		Source string `json:"source"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.Source != "demo_csv" {
		return invalid("source", "Поддерживается только demo_csv")
	}
	if strings.TrimSpace(s.Config.ImportCSVPath) == "" {
		return problem(http.StatusServiceUnavailable, "IMPORT_SOURCE_UNAVAILABLE", "Источник импорта не настроен")
	}
	item, err := changeJSON(r.Context(), s.DB, `INSERT INTO importer.import_runs(source,status) VALUES($1,'pending') RETURNING id,status`, body.Source)
	if err != nil {
		return err
	}
	return respond(w, http.StatusAccepted, map[string]any{"run_id": item["id"], "status": item["status"]})
}
func (s *Server) listImports(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	page, err := pagination(r)
	if err != nil {
		return err
	}
	status := r.URL.Query().Get("status")
	if status != "" && !oneOf(status, "pending", "running", "completed", "failed") {
		return invalid("status", "Неизвестный статус импорта")
	}
	var total int64
	if err = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM importer.import_runs WHERE ($1='' OR status=$1)`, status).Scan(&total); err != nil {
		return err
	}
	items, err := listJSON(r.Context(), s.DB, `SELECT * FROM importer.import_runs WHERE ($1='' OR status=$1) ORDER BY started_at DESC,id LIMIT $2 OFFSET $3`, status, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return paged(w, items, total, page)
}
func (s *Server) getImport(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.requireRole(r, "moderator"); err != nil {
		return err
	}
	id, err := pathID(r, "run_id")
	if err != nil {
		return err
	}
	item, err := oneJSON(r.Context(), s.DB, `SELECT * FROM importer.import_runs WHERE id=$1`, id)
	if err != nil {
		return err
	}
	return respond(w, http.StatusOK, item)
}

// RunImports drains the durable PostgreSQL queue. Each bounded CSV run owns its
// row lock until COMMIT. SKIP LOCKED permits several workers without duplicate
// claims. A crash/cancellation rolls the transaction back to pending, so there
// is no in-memory queue or stranded running lease to recover. For this MVP,
// running/progress are transactional and become externally visible at commit.
func (s *Server) RunImports(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		found, err := s.processNextImport(runCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if found && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func rollbackImport(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func (s *Server) processNextImport(ctx context.Context) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollbackImport(tx)
	var runID, source string
	err = tx.QueryRow(ctx, `SELECT id::text,source FROM importer.import_runs WHERE status='pending' ORDER BY started_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&runID, &source)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE importer.import_runs SET status='running',processed_count=0,error_message=NULL,finished_at=NULL WHERE id=$1`, runID); err != nil {
		return true, err
	}
	// Any validation/record failure rolls back every record, while retaining the
	// claimed run so the final safe error can be committed atomically.
	if _, err = tx.Exec(ctx, `SAVEPOINT import_records`); err != nil {
		return true, err
	}
	records, runErr := s.loadImportCSV(ctx, source)
	processed := 0
	if runErr == nil {
		for _, record := range records {
			if err = ctx.Err(); err != nil {
				return true, err
			}
			if err = s.importCompany(ctx, tx, runID, source, record); err != nil {
				runErr = importFailure("Не удалось сохранить данные импорта")
				break
			}
			processed++
		}
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	if runErr != nil {
		if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT import_records`); err != nil {
			return true, err
		}
		if _, err = tx.Exec(ctx, `UPDATE importer.import_runs SET status='failed',processed_count=0,error_message=$2,finished_at=now() WHERE id=$1`, runID, safeImportMessage(runErr)); err != nil {
			return true, err
		}
	} else {
		if _, err = tx.Exec(ctx, `UPDATE importer.import_runs SET status='completed',processed_count=$2,error_message=NULL,finished_at=now() WHERE id=$1`, runID, processed); err != nil {
			return true, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return true, err
	}
	return true, nil
}

type importFailure string

func (e importFailure) Error() string { return string(e) }
func safeImportMessage(err error) string {
	var known importFailure
	if errors.As(err, &known) {
		return string(known)
	}
	return "Не удалось выполнить импорт"
}

type importRecord struct {
	ExternalID    string
	Name          string
	CategoryID    string
	City          string
	Address       string
	Phone         string
	Email         string
	WebsiteURL    string
	WebsiteStatus string
	SourceURL     string
	Raw           map[string]string
}

func (s *Server) loadImportCSV(ctx context.Context, source string) ([]importRecord, error) {
	if source != "demo_csv" {
		return nil, importFailure("Неизвестный источник импорта")
	}
	if strings.TrimSpace(s.Config.ImportCSVPath) == "" {
		return nil, importFailure("Источник импорта не настроен")
	}
	// The path is server configuration only; no client-controlled file or URL.
	stat, err := os.Stat(s.Config.ImportCSVPath)
	if err != nil || !stat.Mode().IsRegular() {
		return nil, importFailure("Источник импорта недоступен")
	}
	if stat.Size() > importMaxBytes {
		return nil, importFailure("CSV превышает ограничение 20 МБ")
	}
	f, err := os.Open(s.Config.ImportCSVPath)
	if err != nil {
		return nil, importFailure("Источник импорта недоступен")
	}
	defer f.Close()
	return parseImportCSV(ctx, f)
}
func parseImportCSV(ctx context.Context, input io.Reader) ([]importRecord, error) {
	limited := &io.LimitedReader{R: input, N: importMaxBytes + 1}
	reader := csv.NewReader(limited)
	header, err := reader.Read()
	if err != nil {
		return nil, importFailure("Не удалось прочитать заголовок CSV")
	}
	allowed := map[string]bool{"external_id": true, "name": true, "category_id": true, "city": true, "address": true, "phone": true, "email": true, "website_url": true, "website_status": true, "source_url": true}
	seenHeader := map[string]bool{}
	for i, col := range header {
		col = strings.TrimSpace(strings.TrimPrefix(col, "\ufeff"))
		header[i] = col
		if !allowed[col] || seenHeader[col] {
			return nil, importFailure("CSV содержит неизвестные или повторяющиеся столбцы")
		}
		seenHeader[col] = true
	}
	if !seenHeader["external_id"] || !seenHeader["name"] {
		return nil, importFailure("В CSV обязательны столбцы external_id и name")
	}
	records := make([]importRecord, 0)
	seenID := map[string]bool{}
	for row := 2; ; row++ {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		values, readErr := reader.Read()
		if limited.N <= 0 {
			return nil, importFailure("CSV превышает ограничение 20 МБ")
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, importFailure(fmt.Sprintf("Некорректный CSV в строке %d", row))
		}
		if len(records) >= importMaxRows {
			return nil, importFailure("CSV превышает ограничение 10000 записей")
		}
		raw := map[string]string{}
		for i, col := range header {
			raw[col] = values[i]
		}
		record := importRecord{ExternalID: strings.TrimSpace(raw["external_id"]), Name: strings.Join(strings.Fields(raw["name"]), " "), CategoryID: strings.TrimSpace(raw["category_id"]), City: strings.Join(strings.Fields(raw["city"]), " "), Address: strings.TrimSpace(raw["address"]), Phone: strings.TrimSpace(raw["phone"]), Email: strings.ToLower(strings.TrimSpace(raw["email"])), WebsiteURL: strings.TrimSpace(raw["website_url"]), WebsiteStatus: strings.TrimSpace(raw["website_status"]), SourceURL: strings.TrimSpace(raw["source_url"]), Raw: raw}
		if record.City == "" {
			record.City = "Астана"
		}
		if record.WebsiteStatus == "" {
			record.WebsiteStatus = "needs_review"
		}
		if err = record.validate(); err != nil {
			return nil, importFailure(fmt.Sprintf("Некорректные данные CSV в строке %d", row))
		}
		if seenID[record.ExternalID] {
			return nil, importFailure(fmt.Sprintf("Повтор external_id в строке %d", row))
		}
		seenID[record.ExternalID] = true
		records = append(records, record)
	}
	// Deterministic locking order avoids reverse-order upsert deadlocks when two
	// independent runs update the same source records concurrently.
	sort.Slice(records, func(i, j int) bool { return records[i].ExternalID < records[j].ExternalID })
	return records, nil
}
func (r importRecord) validate() error {
	if r.ExternalID == "" || utf8.RuneCountInString(r.ExternalID) > 200 {
		return errors.New("external_id")
	}
	if r.Name == "" || utf8.RuneCountInString(r.Name) > 300 {
		return errors.New("name")
	}
	if r.CategoryID != "" && !validUUID(r.CategoryID) {
		return errors.New("category_id")
	}
	if utf8.RuneCountInString(r.City) > 200 || utf8.RuneCountInString(r.Address) > 1000 || utf8.RuneCountInString(r.Phone) > 100 || len(r.Email) > 320 || len(r.WebsiteURL) > 2048 || len(r.SourceURL) > 2048 {
		return errors.New("length")
	}
	if !oneOf(r.WebsiteStatus, "not_found", "found", "needs_review") {
		return errors.New("website_status")
	}
	if r.Email != "" {
		address, err := mail.ParseAddress(r.Email)
		if err != nil || address.Address != r.Email {
			return errors.New("email")
		}
	}
	if err := validateURL("website_url", r.WebsiteURL); err != nil {
		return err
	}
	return validateURL("source_url", r.SourceURL)
}
func (s *Server) importCompany(ctx context.Context, tx pgx.Tx, runID, source string, r importRecord) error {
	raw, err := json.Marshal(r.Raw)
	if err != nil {
		return err
	}
	var recordID string
	var companyID *string
	// Conflict upsert acquires the source-record lock before any company INSERT,
	// so concurrent/repeated imports cannot create duplicate companies for a key.
	err = tx.QueryRow(ctx, `INSERT INTO importer.source_records(source,external_id,last_run_id,raw_data,last_seen_at) VALUES($1,$2,$3,$4,now()) ON CONFLICT(source,external_id) DO UPDATE SET last_run_id=EXCLUDED.last_run_id,raw_data=EXCLUDED.raw_data,last_seen_at=now() RETURNING id::text,company_id::text`, source, r.ExternalID, runID, raw).Scan(&recordID, &companyID)
	if err != nil {
		return err
	}
	args := []any{r.Name, r.CategoryID, r.City, r.Address, r.Phone, r.Email, r.WebsiteURL, r.WebsiteStatus, r.SourceURL}
	if companyID == nil {
		var id string
		err = tx.QueryRow(ctx, `INSERT INTO catalog.companies(name,category_id,city,address,phone,email,website_url,website_status,source_url) VALUES($1,NULLIF($2,'')::uuid,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,NULLIF($9,'')) RETURNING id::text`, args...).Scan(&id)
		if err != nil {
			return err
		}
		companyID = &id
		_, err = tx.Exec(ctx, `UPDATE importer.source_records SET company_id=$2 WHERE id=$1`, recordID, id)
		return err
	}
	args = append(args, *companyID)
	_, err = tx.Exec(ctx, `UPDATE catalog.companies SET name=$1,category_id=NULLIF($2,'')::uuid,city=$3,address=NULLIF($4,''),phone=NULLIF($5,''),email=NULLIF($6,''),website_url=NULLIF($7,''),website_status=$8,source_url=NULLIF($9,''),updated_at=now() WHERE id=$10`, args...)
	// Preserve an existing moderation visibility decision when refreshing source data.
	return err
}
