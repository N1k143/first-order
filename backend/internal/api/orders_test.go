package api

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func orderTestNumber(v float64) *float64 { return &v }
func orderTestString(v string) *string   { return &v }

func TestOrderPatchDistinguishesMissingNullAndZero(t *testing.T) {
	var d orderPatchDTO
	if err := json.Unmarshal([]byte(`{"budget_min":null,"budget_max":0,"deadline":null}`), &d); err != nil {
		t.Fatal(err)
	}
	if !d.BudgetMin.Set || d.BudgetMin.Value != nil {
		t.Fatal("explicit null must clear budget_min")
	}
	if !d.BudgetMax.Set || d.BudgetMax.Value == nil || *d.BudgetMax.Value != 0 {
		t.Fatal("zero budget must not be treated as omitted")
	}
	if !d.Deadline.Set || d.Deadline.Value != nil || d.Title.Set || d.Description.Set {
		t.Fatal("omitted fields must remain unchanged while explicit null clears the date")
	}
	var bad orderPatchDTO
	if err := json.Unmarshal([]byte(`{"budget_min":"100"}`), &bad); err == nil {
		t.Fatal("JSON money strings must not silently coerce into numbers")
	}
}
func TestOrderDTORejectsActorAndStateInjection(t *testing.T) {
	for _, body := range []string{
		`{"title":"A","created_by":"00000000-0000-0000-0000-000000000000"}`,
		`{"title":"A","actor_id":"00000000-0000-0000-0000-000000000000"}`,
		`{"title":"A","status":"completed"}`,
		`{"title":"A","visibility":true}`,
		`{"title":"A","selected_application_id":"00000000-0000-0000-0000-000000000000"}`,
	} {
		t.Run(body, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/orders", strings.NewReader(body))
			var d orderCreateDTO
			if err := readJSON(r, &d); err == nil {
				t.Fatal("server-owned fields must be rejected by canonical DTO")
			}
		})
	}
	for _, body := range []string{
		`{"message":"hello","developer_id":"00000000-0000-0000-0000-000000000000"}`,
		`{"message":"hello","status":"accepted"}`,
		`{"message":"hello","order_id":"00000000-0000-0000-0000-000000000000"}`,
	} {
		r := httptest.NewRequest("POST", "/orders/x/applications", strings.NewReader(body))
		var d orderApplicationCreateDTO
		if err := readJSON(r, &d); err == nil {
			t.Fatalf("application accepted injected ownership or state: %s", body)
		}
	}
}
func TestOrderBudgetAndPriceBoundaries(t *testing.T) {
	for _, v := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := orderBudgets(orderTestNumber(v), nil); err == nil {
			t.Errorf("invalid budget accepted: %v", v)
		}
	}
	if err := orderBudgets(orderTestNumber(150), orderTestNumber(100)); err == nil {
		t.Fatal("inverted budget interval accepted")
	}
	for _, v := range [][2]*float64{{nil, nil}, {nil, orderTestNumber(100)}, {orderTestNumber(100), nil}, {orderTestNumber(0), orderTestNumber(0)}} {
		if err := orderBudgets(v[0], v[1]); err != nil {
			t.Errorf("valid nullable interval rejected: %v", err)
		}
	}
	for _, v := range []float64{0, 0.01, 150000.99, 9999999999.99} {
		if err := orderMoney("proposed_price", &v, true); err != nil {
			t.Errorf("valid numeric(12,2) price %v rejected: %v", v, err)
		}
	}
	for _, v := range []float64{10000000000, 1.001, -0.01} {
		if err := orderMoney("proposed_price", &v, true); err == nil {
			t.Errorf("invalid numeric(12,2) price accepted: %v", v)
		}
	}
}
func TestOrderDatesValidateCalendarAndFutureInput(t *testing.T) {
	for _, v := range []string{"2026-02-29", "2026-13-01", "2026-1-01", "2026-11-19T00:00:00Z", "", "0000-01-01"} {
		if err := orderDate("deadline", &v, false); err == nil {
			t.Errorf("invalid calendar date accepted: %q", v)
		}
	}
	if err := orderDate("deadline", orderTestString("2024-02-29"), false); err != nil {
		t.Fatal("valid past dates are allowed as search filters", err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if err := orderDate("deadline", &today, true); err != nil {
		t.Fatal("today must be a valid submission deadline", err)
	}
	if err := orderDate("deadline", &yesterday, true); err == nil {
		t.Fatal("new deadlines in the past must be rejected")
	}
	if err := orderDate("deadline", nil, true); err != nil {
		t.Fatal("nullable dates can be cleared", err)
	}
}
func TestOrderFilterPreservesACLAndParameterizesInput(t *testing.T) {
	r := httptest.NewRequest("GET", "/orders?status=closed&q=%27%20OR%201%3D1--&company_id=12345678-1234-1234-1234-123456789abc", nil)
	where, args, err := orderFilter(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(where, orderReadACL+" AND ") || strings.Contains(where, "OR 1=1--") {
		t.Fatal("filters must retain ACL and use SQL parameters")
	}
	if len(args) != 5 || args[1] != false || args[2] != "' OR 1=1--" {
		t.Fatalf("unexpected parameter binding: %#v", args)
	}
	if strings.Contains(where, "%!") || !strings.Contains(where, "o.description ILIKE '%' || $3 || '%'") {
		t.Fatalf("invalid search SQL placeholders: %s", where)
	}
	if _, _, err := orderFilter(httptest.NewRequest("GET", "/orders?mine=true", nil), nil); err == nil {
		t.Fatal("anonymous callers cannot request a company workspace")
	}
	actor := &Actor{ID: "12345678-1234-1234-1234-123456789abc", Role: "developer"}
	where, _, err = orderFilter(httptest.NewRequest("GET", "/orders?mine=true", nil), actor)
	if err != nil || !strings.Contains(where, "own.company_id=o.company_id AND own.user_id=$1::uuid") {
		t.Fatal("workspace filter must require membership", err)
	}
}
func TestOrderFiltersRejectInvalidBusinessValues(t *testing.T) {
	for _, q := range []string{"status=accepted", "company_id=nope", "budget_from=NaN", "budget_to=Infinity", "budget_from=-1", "budget_from=200&budget_to=100", "deadline_before=2026-02-30", "mine=maybe"} {
		if _, _, err := orderFilter(httptest.NewRequest("GET", "/orders?"+q, nil), nil); err == nil {
			t.Errorf("invalid filter accepted: %s", q)
		}
	}
}
func TestOrderCreateNormalizesRequiredText(t *testing.T) {
	d := orderCreateDTO{CompanyID: "12345678-1234-1234-1234-123456789abc", Title: "  Сайт  ", Description: "  Описание задачи \n"}
	if err := d.validate(); err != nil {
		t.Fatal(err)
	}
	if d.Title != "Сайт" || d.Description != "Описание задачи" {
		t.Fatal("required text must be trimmed")
	}
	d.Title = "\t \n"
	if err := d.validate(); err == nil {
		t.Fatal("whitespace-only title accepted")
	}
}
func TestOrderActionsRejectInjectedBodies(t *testing.T) {
	for _, body := range []string{`{"actor_id":"another-user"}`, `{"status":"open"}`, `{} {}`} {
		if err := orderEmptyBody(httptest.NewRequest("POST", "/orders/x/publish", strings.NewReader(body))); err == nil {
			t.Errorf("action accepted extraneous data: %s", body)
		}
	}
	for _, body := range []string{"", `{}`} {
		if err := orderEmptyBody(httptest.NewRequest("POST", "/orders/x/publish", strings.NewReader(body))); err != nil {
			t.Errorf("action rejected valid empty body: %q: %v", body, err)
		}
	}
}

func orderTestCreate(t *testing.T, s *Server, f testFixture) string {
	t.Helper()
	res := testRequest(t, s, "POST", "/api/v1/orders", f.Owner, map[string]any{"company_id": f.Company, "title": "Integration order", "description": "A complete test description", "budget_min": 100, "budget_max": 200, "deadline": time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")})
	testStatus(t, res, 201)
	return testObject(t, res)["id"].(string)
}
func orderTestApplication(t *testing.T, s *Server, orderID, developer string) string {
	t.Helper()
	res := testRequest(t, s, "POST", "/api/v1/orders/"+orderID+"/applications", developer, map[string]any{"message": "A useful test proposal", "proposed_price": 150.25, "proposed_deadline": time.Now().UTC().AddDate(0, 0, 20).Format("2006-01-02")})
	testStatus(t, res, 201)
	return testObject(t, res)["id"].(string)
}
func TestOrdersIntegrationLifecycleAndACL(t *testing.T) {
	s, pool, f := dbTest(t)
	id := orderTestCreate(t, s, f)
	base := "/api/v1/orders/" + id
	testStatus(t, testRequest(t, s, "GET", base, "", nil), 404)
	testStatus(t, testRequest(t, s, "GET", base, f.OtherDeveloper, nil), 404)
	testStatus(t, testRequest(t, s, "GET", base, f.Owner, nil), 200)
	testStatus(t, testRequest(t, s, "GET", base, f.Moderator, nil), 200)
	res := testRequest(t, s, "GET", "/api/v1/orders?company_id="+f.Company, "", nil)
	testStatus(t, res, 200)
	if testObject(t, res)["total"] != float64(0) {
		t.Fatal("anonymous list exposed a draft")
	}
	// Merged validation allows clearing an upper bound, but rejects an inverted range.
	res = testRequest(t, s, "PATCH", base, f.Owner, map[string]any{"budget_min": 250, "budget_max": nil})
	testStatus(t, res, 200)
	if testObject(t, res)["budget_max"] != nil {
		t.Fatal("PATCH null did not clear budget_max")
	}
	testStatus(t, testRequest(t, s, "PATCH", base, f.Owner, map[string]any{"budget_max": 100}), 422)
	testStatus(t, testRequest(t, s, "PATCH", base, f.OtherDeveloper, map[string]any{"title": "Hijacked"}), 403)
	testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Moderator, nil), 403)
	testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
	testStatus(t, testRequest(t, s, "GET", base, "", nil), 200)
	testStatus(t, testRequest(t, s, "POST", base+"/applications", f.Owner, map[string]any{"message": "I represent this company"}), 403)
	app := orderTestApplication(t, s, id, f.Developer)
	res = testRequest(t, s, "PATCH", "/api/v1/applications/"+app, f.Developer, map[string]any{"message": "Updated proposal", "proposed_price": nil, "proposed_deadline": nil})
	testStatus(t, res, 200)
	if testObject(t, res)["proposed_price"] != nil || testObject(t, res)["proposed_deadline"] != nil {
		t.Fatal("nullable application fields were not cleared")
	}
	testStatus(t, testRequest(t, s, "GET", "/api/v1/applications/"+app, f.Developer, nil), 200)
	testStatus(t, testRequest(t, s, "GET", "/api/v1/me/applications", f.Developer, nil), 200)
	withdrawn := orderTestApplication(t, s, id, f.OtherDeveloper)
	testStatus(t, testRequest(t, s, "POST", base+"/applications", f.Developer, map[string]any{"message": "Duplicate"}), 409)
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/applications/"+app, f.OtherDeveloper, map[string]any{"message": "Hijacked"}), 403)
	testStatus(t, testRequest(t, s, "GET", base+"/applications", f.Developer, nil), 403)
	testStatus(t, testRequest(t, s, "GET", base+"/applications", f.Owner, nil), 200)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/applications/"+withdrawn+"/withdraw", f.OtherDeveloper, nil), 200)
	testStatus(t, testRequest(t, s, "POST", base+"/applications", f.OtherDeveloper, map[string]any{"message": "Cannot reapply after withdrawal"}), 409)
	testStatus(t, testRequest(t, s, "POST", base+"/select-application", f.Moderator, map[string]any{"application_id": app}), 403)
	res = testRequest(t, s, "POST", base+"/select-application", f.Owner, map[string]any{"application_id": app})
	testStatus(t, res, 200)
	if testObject(t, res)["status"] != "contractor_selected" {
		t.Fatal("selection did not advance the order")
	}
	testStatus(t, testRequest(t, s, "GET", base, "", nil), 404)
	testStatus(t, testRequest(t, s, "GET", base, f.Developer, nil), 200)
	testStatus(t, testRequest(t, s, "GET", base, f.OtherDeveloper, nil), 404)
	testStatus(t, testRequest(t, s, "PATCH", "/api/v1/applications/"+app, f.Developer, map[string]any{"message": "Too late"}), 409)
	var chosenState, withdrawnState string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM applications.applications WHERE id=$1`, app).Scan(&chosenState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT status FROM applications.applications WHERE id=$1`, withdrawn).Scan(&withdrawnState); err != nil {
		t.Fatal(err)
	}
	if chosenState != "accepted" || withdrawnState != "withdrawn" {
		t.Fatalf("incorrect application states: %s %s", chosenState, withdrawnState)
	}
	testStatus(t, testRequest(t, s, "POST", base+"/complete", f.Owner, nil), 200)
	testStatus(t, testRequest(t, s, "POST", base+"/complete", f.Owner, nil), 409)
	testStatus(t, testRequest(t, s, "GET", base, f.Developer, nil), 200)
}

func TestOrdersIntegrationCloseAndVisibility(t *testing.T) {
	s, pool, f := dbTest(t)
	id := orderTestCreate(t, s, f)
	base := "/api/v1/orders/" + id
	testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
	app := orderTestApplication(t, s, id, f.Developer)
	testStatus(t, testRequest(t, s, "PATCH", base+"/visibility", f.Owner, map[string]any{"visibility": false}), 403)
	testStatus(t, testRequest(t, s, "PATCH", base+"/visibility", f.Moderator, map[string]any{"visibility": false, "reason": "Review"}), 200)
	testStatus(t, testRequest(t, s, "GET", base, "", nil), 404)
	testStatus(t, testRequest(t, s, "POST", base+"/applications", f.OtherDeveloper, map[string]any{"message": "Hidden order"}), 404)
	testStatus(t, testRequest(t, s, "PATCH", base+"/visibility", f.Moderator, map[string]any{"visibility": true}), 200)
	testStatus(t, testRequest(t, s, "POST", base+"/close", f.Moderator, nil), 200)
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM applications.applications WHERE id=$1`, app).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Fatal("close must reject outstanding submitted applications")
	}
	// Restoring visibility cannot reopen the lifecycle.
	res := testRequest(t, s, "PATCH", base+"/visibility", f.Moderator, map[string]any{"visibility": true})
	testStatus(t, res, 200)
	if testObject(t, res)["status"] != "closed" {
		t.Fatal("visibility restoration reopened a closed order")
	}
	var audits int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM catalog.moderation_actions WHERE order_id=$1`, id).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("expected hide/publish audit only, got %d", audits)
	}
	selectedOrder := orderTestCreate(t, s, f)
	selectedBase := "/api/v1/orders/" + selectedOrder
	testStatus(t, testRequest(t, s, "POST", selectedBase+"/publish", f.Owner, nil), 200)
	accepted := orderTestApplication(t, s, selectedOrder, f.Developer)
	testStatus(t, testRequest(t, s, "POST", selectedBase+"/select-application", f.Owner, map[string]any{"application_id": accepted}), 200)
	res = testRequest(t, s, "POST", selectedBase+"/close", f.Owner, nil)
	testStatus(t, res, 200)
	if testObject(t, res)["selected_application_id"] != accepted {
		t.Fatal("close discarded contractor history")
	}
	if err := pool.QueryRow(t.Context(), `SELECT status FROM applications.applications WHERE id=$1`, accepted).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "accepted" {
		t.Fatal("close must preserve accepted history")
	}
}

func TestOrdersIntegrationConcurrentSelectionAndWithdrawal(t *testing.T) {
	s, pool, f := dbTest(t)
	for _, scenario := range []string{"two_selections", "selection_withdrawal"} {
		t.Run(scenario, func(t *testing.T) {
			id := orderTestCreate(t, s, f)
			base := "/api/v1/orders/" + id
			testStatus(t, testRequest(t, s, "POST", base+"/publish", f.Owner, nil), 200)
			first := orderTestApplication(t, s, id, f.Developer)
			second := orderTestApplication(t, s, id, f.OtherDeveloper)
			type result struct {
				code int
				body string
			}
			results := make(chan result, 2)
			start := make(chan struct{})
			go func() {
				<-start
				res := testRequest(t, s, "POST", base+"/select-application", f.Owner, map[string]any{"application_id": first})
				results <- result{res.Code, res.Body.String()}
			}()
			go func() {
				<-start
				var res *httptest.ResponseRecorder
				if scenario == "two_selections" {
					res = testRequest(t, s, "POST", base+"/select-application", f.Owner, map[string]any{"application_id": second})
				} else {
					res = testRequest(t, s, "POST", "/api/v1/applications/"+first+"/withdraw", f.Developer, nil)
				}
				results <- result{res.Code, res.Body.String()}
			}()
			close(start)
			a, b := <-results, <-results
			if !((a.code == 200 && b.code == 409) || (a.code == 409 && b.code == 200)) {
				t.Fatalf("expected one success and one conflict: %#v %#v", a, b)
			}
			var accepted int
			var status string
			var selected *string
			if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM applications.applications WHERE order_id=$1 AND status='accepted'`, id).Scan(&accepted); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(t.Context(), `SELECT status,selected_application_id::text FROM orders.orders WHERE id=$1`, id).Scan(&status, &selected); err != nil {
				t.Fatal(err)
			}
			if scenario == "two_selections" && accepted != 1 {
				t.Fatalf("double selection produced %d accepted replies", accepted)
			}
			if status == "contractor_selected" {
				if selected == nil || accepted != 1 {
					t.Fatal("selected order and accepted reply diverged")
				}
				var state string
				if err := pool.QueryRow(t.Context(), `SELECT status FROM applications.applications WHERE id=$1`, *selected).Scan(&state); err != nil {
					t.Fatal(err)
				}
				if state != "accepted" {
					t.Fatalf("selected reply became %s", state)
				}
			} else if status != "open" || selected != nil || accepted != 0 {
				t.Fatalf("invalid state after withdrawal won: %s %v %d", status, selected, accepted)
			}
		})
	}
}

func TestOrdersIntegrationBackendRole(t *testing.T) {
	s, pool, f := dbTest(t)
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err = conn.Exec(t.Context(), `SET ROLE firstorder_backend`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(t.Context(), `RESET ROLE`)
	s.DB = conn.Conn()
	id := orderTestCreate(t, s, f)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/orders/"+id+"/publish", f.Owner, nil), 200)
	app := orderTestApplication(t, s, id, f.Developer)
	testStatus(t, testRequest(t, s, "POST", "/api/v1/orders/"+id+"/select-application", f.Owner, map[string]any{"application_id": app}), 200)
}
