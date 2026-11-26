package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func notificationCount(t *testing.T, pool *pgxpool.Pool, actor, order, event string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM profiles.notifications WHERE user_id=$1 AND order_id=$2 AND event=$3`, actor, order, event).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestUserWorkflowsFullLifecycle(t *testing.T) {
	s, pool, f := dbTest(t)
	// Use the same privileges as the deployed API throughout the scenario.
	cfg := pool.Config().Copy()
	cfg.ConnConfig.RuntimeParams["role"] = "firstorder_backend"
	runtime, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	s.DB = runtime
	id := orderTestCreate(t, s, f)
	base := "/api/v1/orders/" + id
	testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
	testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
	testStatus(t, testRequest(t, s, "GET", base+"/participants", f.Owner, nil), 404)
	chosen := orderTestApplication(t, s, id, f.Developer)
	other := orderTestApplication(t, s, id, f.OtherDeveloper)
	testStatus(t, testRequest(t, s, "GET", base+"/participants", f.Developer, nil), 404)
	if notificationCount(t, pool, f.Owner, id, "application_submitted") != 2 {
		t.Fatal("company did not receive applications")
	}
	rec := testRequest(t, s, "GET", "/api/v1/me/company-applications?company_id="+f.Company+"&status=submitted", f.Owner, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["total"] != float64(2) {
		t.Fatal("company application inbox incomplete")
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/me/company-applications?company_id="+f.Company, f.Developer, nil), 403)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/applications/"+other+"/reject", f.OtherDeveloper, nil), 403)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/applications/"+other+"/reject", f.Owner, nil), 200)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/applications/"+other+"/reject", f.Owner, nil), 200)
	if notificationCount(t, pool, f.OtherDeveloper, id, "application_rejected") != 1 {
		t.Fatal("duplicate or missing rejection notification")
	}
	testStatus(t, testRequest(t, s, "POST", base+"/select-application", f.Owner, map[string]any{"application_id": other}), 409)
	for i := 0; i < 2; i++ {
		testStatus(t, testRequest(t, s, "POST", base+"/select-application", f.Owner, map[string]any{"application_id": chosen}), 200)
	}
	if notificationCount(t, pool, f.Developer, id, "application_accepted") != 1 {
		t.Fatal("selection notification duplicated")
	}
	for _, actor := range []string{f.Owner, f.Developer} {
		rec = testRequest(t, s, "GET", base+"/participants", actor, nil)
		testStatus(t, rec, 200)
		obj := testObject(t, rec)
		if obj["contractor"].(map[string]any)["email"] != f.Developer+"@integration.invalid" || obj["customer"].(map[string]any)["email"] != f.Owner+"@integration.invalid" {
			t.Fatal("participant contacts incorrect")
		}
		if rec.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("private contacts are cacheable")
		}
	}
	for _, actor := range []string{"", f.OtherDeveloper, f.Moderator} {
		want := 404
		if actor == "" {
			want = 401
		}
		testStatus(t, testRequest(t, s, "GET", base+"/participants", actor, nil), want)
	}
	testStatus(t, testRequest(t, s, "POST", "/api/v1/applications/"+chosen+"/reject", f.Owner, nil), 409)
	testStatus(t, testRequest(t, s, "POST", base+"/complete", f.Developer, nil), 403)
	for i := 0; i < 2; i++ {
		testStatus(t, testRequest(t, s, "POST", base+"/complete", f.Owner, nil), 200)
	}
	if notificationCount(t, pool, f.Developer, id, "order_completed") != 1 {
		t.Fatal("completion notification duplicated")
	}
	rec = testRequest(t, s, "GET", "/api/v1/applications/"+chosen, f.Developer, nil)
	testStatus(t, rec, 200)
	obj := testObject(t, rec)
	if obj["status"] != "accepted" || obj["order_status"] != "completed" || obj["is_selected"] != true {
		t.Fatal("completed work lost application history")
	}
	rec = testRequest(t, s, "GET", "/applications/"+chosen, f.Developer, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["status"] != "completed" {
		t.Fatal("legacy frontend still sees selected after completion")
	}
	rec = testRequest(t, s, "GET", "/api/v1/me/work?status=completed", f.Developer, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["total"] != float64(1) {
		t.Fatal("completed work missing")
	}
	rec = testRequest(t, s, "GET", "/api/v1/me/work", f.OtherDeveloper, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["total"] != float64(0) {
		t.Fatal("other applicant can see assigned work")
	}
	rec = testRequest(t, s, "GET", "/api/v1/me/workspace", f.Developer, nil)
	testStatus(t, rec, 200)
	obj = testObject(t, rec)
	if obj["work"].(map[string]any)["completed"] != float64(1) || obj["applications"].(map[string]any)["accepted"] != float64(1) {
		t.Fatal("developer counters incorrect")
	}
	rec = testRequest(t, s, "GET", "/api/v1/me/workspace", f.Owner, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["orders"].(map[string]any)["completed"] != float64(1) {
		t.Fatal("company counters incorrect")
	}
}

func TestUserWorkflowsNotificationOwnershipAndRead(t *testing.T) {
	s, _, f := dbTest(t)
	id := orderTestCreate(t, s, f)
	base := "/api/v1/orders/" + id
	testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
	app := orderTestApplication(t, s, id, f.Developer)
	testStatus(t, testRequest(t, s, "POST", base+"/select-application", f.Owner, map[string]any{"application_id": app}), 200)
	rec := testRequest(t, s, "GET", "/api/v1/me/notifications?unread=true", f.Developer, nil)
	testStatus(t, rec, 200)
	obj := testObject(t, rec)
	items := obj["items"].([]any)
	if len(items) != 1 || obj["unread_count"] != float64(1) {
		t.Fatal("unread inbox incorrect")
	}
	notification := items[0].(map[string]any)["id"].(string)
	path := "/api/v1/me/notifications/" + notification + "/read"
	testStatus(t, testRequest(t, s, "POST", path, f.OtherDeveloper, nil), 404)
	testStatus(t, testRequest(t, s, "POST", path, f.Moderator, nil), 404)
	first := testRequest(t, s, "POST", path, f.Developer, nil)
	testStatus(t, first, 200)
	repeated := testRequest(t, s, "POST", path, f.Developer, nil)
	testStatus(t, repeated, 200)
	if testObject(t, first)["read_at"] != testObject(t, repeated)["read_at"] {
		t.Fatal("read replay changed timestamp")
	}
	testStatus(t, testRequest(t, s, "POST", base+"/complete", f.Owner, nil), 200)
	rec = testRequest(t, s, "POST", "/api/v1/me/notifications/read-all", f.Developer, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["updated"] != float64(1) {
		t.Fatal("bulk read touched unrelated/already-read rows")
	}
	rec = testRequest(t, s, "GET", "/api/v1/me/notifications?unread=true", f.Owner, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["unread_count"] != float64(1) {
		t.Fatal("bulk read modified another recipient")
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/me/notifications?unread=invalid", f.Developer, nil), 422)
	testStatus(t, testRequest(t, s, "POST", path, f.Developer, map[string]any{"user_id": f.Owner}), 400)
}

func TestUserWorkflowsCloseAndWithdrawal(t *testing.T) {
	s, pool, f := dbTest(t)
	for _, selected := range []bool{false, true} {
		id := orderTestCreate(t, s, f)
		base := "/api/v1/orders/" + id
		testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
		app := orderTestApplication(t, s, id, f.Developer)
		if selected {
			testStatus(t, testRequest(t, s, "POST", base+"/select-application", f.Owner, map[string]any{"application_id": app}), 200)
		}
		for i := 0; i < 2; i++ {
			testStatus(t, testRequest(t, s, "POST", base+"/close", f.Owner, nil), 200)
		}
		event := "application_rejected"
		if selected {
			event = "order_closed"
		}
		if notificationCount(t, pool, f.Developer, id, event) != 1 {
			t.Fatal("closing notification missing or duplicated")
		}
		testStatus(t, testRequest(t, s, "GET", base+"/participants", f.Developer, nil), 404)
		testStatus(t, testRequest(t, s, "POST", base+"/complete", f.Owner, nil), 409)
	}
	id := orderTestCreate(t, s, f)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/orders/"+id+"/publish", f.Owner, nil), 200)
	app := orderTestApplication(t, s, id, f.Developer)
	for i := 0; i < 2; i++ {
		testStatus(t, testRequest(t, s, "POST", "/api/v1/applications/"+app+"/withdraw", f.Developer, nil), 200)
	}
	if notificationCount(t, pool, f.Owner, id, "application_withdrawn") != 1 {
		t.Fatal("withdrawal notification duplicated")
	}
}

func TestUserWorkflowsConcurrentDecision(t *testing.T) {
	s, pool, f := dbTest(t)
	id := orderTestCreate(t, s, f)
	base := "/api/v1/orders/" + id
	testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
	app := orderTestApplication(t, s, id, f.Developer)
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for _, selecting := range []bool{true, false} {
		wg.Add(1)
		go func(selecting bool) {
			defer wg.Done()
			<-start
			if selecting {
				results <- testRequest(t, s, "POST", base+"/select-application", f.Owner, map[string]any{"application_id": app})
			} else {
				results <- testRequest(t, s, "POST", "/api/v1/applications/"+app+"/reject", f.Owner, nil)
			}
		}(selecting)
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for rec := range results {
		if rec.Code == 200 {
			success++
		} else {
			testStatus(t, rec, 409)
		}
	}
	if success != 1 {
		t.Fatal("conflicting decisions both succeeded")
	}
	if notificationCount(t, pool, f.Developer, id, "application_accepted")+notificationCount(t, pool, f.Developer, id, "application_rejected") != 1 {
		t.Fatal("recipient received conflicting decisions")
	}
}

func TestUserWorkflowsNotificationFailureRollsBackApplication(t *testing.T) {
	s, pool, f := dbTest(t)
	id := orderTestCreate(t, s, f)
	base := "/api/v1/orders/" + id
	testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
	suffix := strings.ReplaceAll(newID(), "-", "")
	function := "profiles.fail_notification_" + suffix
	trigger := "test_notification_" + suffix
	sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced notification failure'; END $$; CREATE TRIGGER %s BEFORE INSERT ON profiles.notifications FOR EACH ROW WHEN (NEW.order_id='%s'::uuid) EXECUTE FUNCTION %s();`, function, trigger, id, function)
	if _, err := pool.Exec(t.Context(), sql); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DROP TRIGGER `+trigger+` ON profiles.notifications; DROP FUNCTION `+function+`();`)
	})
	testStatus(t, testRequest(t, s, "POST", base+"/applications", f.Developer, map[string]any{"message": "Attempt"}), 500)
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM applications.applications WHERE order_id=$1`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("application committed without notification")
	}
}

func TestUserWorkflowsRequireHumanSession(t *testing.T) {
	s, pool, f := dbTest(t)
	s.Config.ImporterToken = "user-workflows-private-at-least-32-characters"
	routes := []struct{ method, path string }{
		{"GET", "/api/v1/me/notifications"},
		{"POST", "/api/v1/me/notifications/read-all"},
		{"POST", "/api/v1/me/notifications/" + newID() + "/read"},
		{"GET", "/api/v1/me/workspace"},
		{"GET", "/api/v1/me/work"},
		{"GET", "/api/v1/me/company-applications"},
		{"GET", "/api/v1/orders/" + newID() + "/participants"},
		{"POST", "/api/v1/applications/" + newID() + "/reject"},
	}
	for _, route := range routes {
		testStatus(t, testRequest(t, s, route.method, route.path, "", nil), 401)
		testStatus(t, testRequestWithImporter(t, s, route.method, route.path, nil), 403)
	}
	for _, role := range []string{"anon", "authenticated"} {
		var allowed bool
		if err := pool.QueryRow(t.Context(), `SELECT has_table_privilege($1,'profiles.notifications','SELECT') OR has_table_privilege($1,'profiles.notifications','UPDATE')`, role).Scan(&allowed); err != nil || allowed {
			t.Fatalf("browser access %s: %v", role, err)
		}
	}
	rec := testRequest(t, s, "GET", "/api/v1/me/workspace", f.OtherDeveloper, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["orders"].(map[string]any)["total"] != float64(0) {
		t.Fatal("workspace leaks company orders")
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/me/work?status=draft", f.Developer, nil), 422)
	testStatus(t, testRequest(t, s, "GET", "/api/v1/me/company-applications?company_id=wrong", f.Owner, nil), 422)
}

func TestUserWorkflowsFormerMemberKeepsOnlyNotificationSnapshot(t *testing.T) {
	s, pool, f := dbTest(t)
	_, err := pool.Exec(t.Context(), `INSERT INTO profiles.company_members(company_id,user_id,member_role) VALUES($1,$2,'representative')`, f.Company, f.OtherDeveloper)
	if err != nil {
		t.Fatal(err)
	}
	id := orderTestCreate(t, s, f)
	base := "/api/v1/orders/" + id
	testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
	orderTestApplication(t, s, id, f.Developer)
	before := testRequest(t, s, "GET", "/api/v1/me/notifications", f.OtherDeveloper, nil)
	testStatus(t, before, 200)
	_, err = pool.Exec(t.Context(), `DELETE FROM profiles.company_members WHERE company_id=$1 AND user_id=$2`, f.Company, f.OtherDeveloper)
	if err != nil {
		t.Fatal(err)
	}
	testStatus(t, testRequest(t, s, "PATCH", base, f.Owner, map[string]any{"title": "Private future title"}), 200)
	after := testRequest(t, s, "GET", "/api/v1/me/notifications", f.OtherDeveloper, nil)
	testStatus(t, after, 200)
	if strings.Contains(after.Body.String(), "Private future title") {
		t.Fatal("revoked member sees later edits")
	}
	if testObject(t, after)["total"] != float64(1) {
		t.Fatal("past notification lost")
	}
	rec := testRequest(t, s, "GET", "/api/v1/me/company-applications", f.OtherDeveloper, nil)
	testStatus(t, rec, 200)
	if testObject(t, rec)["total"] != float64(0) {
		t.Fatal("revoked member sees proposals")
	}
}
