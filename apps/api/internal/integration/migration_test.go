package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMigrationJobCreateAndAddRecords(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mig-crt-"+uuid.NewString()[:8])

	// Create migration job.
	status, body := apiReq(t, "POST", "/v1/migrations", apiKey, map[string]any{
		"source_system": "stripe",
		"dry_run":       false,
	})
	if status != http.StatusCreated {
		t.Fatalf("create job: status %d, body %v", status, body)
	}
	jobID := body["id"].(string)
	if body["status"] != "draft" {
		t.Fatalf("status = %v, want draft", body["status"])
	}

	// Add customer records.
	status, body = apiReq(t, "POST", "/v1/migrations/"+jobID+"/records", apiKey, map[string]any{
		"records": []map[string]any{
			{
				"record_type":  "customer",
				"external_id":  "cus_001",
				"source_data":  map[string]any{"name": "Acme Corp", "email": "billing@acme.com", "type": "business", "external_code": "ext_acme"},
			},
			{
				"record_type":  "customer",
				"external_id":  "cus_002",
				"source_data":  map[string]any{"name": "John Doe", "email": "john@example.com", "type": "individual", "external_code": "ext_john"},
			},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("add records: status %d, body %v", status, body)
	}
	if int(body["added"].(float64)) != 2 {
		t.Fatalf("added = %v, want 2", body["added"])
	}

	// List records.
	status, body = apiReq(t, "GET", "/v1/migrations/"+jobID+"/records", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list records: status %d", status)
	}
	records := body["migration_records"].([]any)
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

func TestMigrationDryRunValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mig-val-"+uuid.NewString()[:8])

	// Create job and add mixed valid/invalid records.
	status, body := apiReq(t, "POST", "/v1/migrations", apiKey, map[string]any{
		"source_system": "chargebee",
	})
	jobID := body["id"].(string)

	apiReq(t, "POST", "/v1/migrations/"+jobID+"/records", apiKey, map[string]any{
		"records": []map[string]any{
			{
				"record_type": "customer",
				"external_id": "valid_001",
				"source_data": map[string]any{"name": "Valid Corp", "email": "v@example.com", "type": "business", "external_code": "ext_valid"},
			},
			{
				"record_type": "customer",
				"external_id": "invalid_001",
				"source_data": map[string]any{"name": "", "email": "no-name@example.com", "type": "business", "external_code": "ext_noname"},
			},
			{
				"record_type": "customer",
				"external_id": "invalid_002",
				"source_data": map[string]any{"name": "Bad Type", "email": "bt@example.com", "type": "enterprise", "external_code": "ext_badtype"},
			},
		},
	})

	// Validate.
	status, body = apiReq(t, "POST", "/v1/migrations/"+jobID+"/validate", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("validate: status %d, body %v", status, body)
	}
	if body["status"] != "validated" {
		t.Fatalf("status = %v, want validated", body["status"])
	}

	// List invalid records.
	status, body = apiReq(t, "GET", "/v1/migrations/"+jobID+"/invalid-records", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("invalid records: status %d", status)
	}
	invalid := body["invalid_records"].([]any)
	if len(invalid) != 2 {
		t.Fatalf("expected 2 invalid records, got %d", len(invalid))
	}
}

func TestMigrationImportAndComplete(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mig-imp-"+uuid.NewString()[:8])

	// Create job and add valid records.
	status, body := apiReq(t, "POST", "/v1/migrations", apiKey, map[string]any{
		"source_system": "stripe",
	})
	jobID := body["id"].(string)

	apiReq(t, "POST", "/v1/migrations/"+jobID+"/records", apiKey, map[string]any{
		"records": []map[string]any{
			{
				"record_type": "customer",
				"external_id": "cus_imp_001",
				"source_data": map[string]any{"name": "Import Corp", "email": "imp@example.com", "type": "business", "external_code": "ext_imp_001"},
			},
		},
	})

	// Validate.
	apiReq(t, "POST", "/v1/migrations/"+jobID+"/validate", apiKey, nil)

	// Start migration (imports records).
	status, body = apiReq(t, "POST", "/v1/migrations/"+jobID+"/start", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("start: status %d, body %v", status, body)
	}
	if body["status"] != "importing" {
		t.Fatalf("status = %v, want importing", body["status"])
	}
	if body["cutover_locked"] != true {
		t.Fatal("cutover_locked should be true during migration")
	}

	// Complete migration.
	status, body = apiReq(t, "POST", "/v1/migrations/"+jobID+"/complete", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("complete: status %d, body %v", status, body)
	}
	if body["status"] != "completed" {
		t.Fatalf("status = %v, want completed", body["status"])
	}
	if body["cutover_locked"] != false {
		t.Fatal("cutover_locked should be false after completion")
	}

	// Verify the customer was imported by listing customers.
	status, body = apiReq(t, "GET", "/v1/customers", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list customers: status %d", status)
	}
	customers := body["customers"].([]any)
	found := false
	for _, c := range customers {
		if c.(map[string]any)["external_id"] == "ext_imp_001" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("imported customer ext_imp_001 not found in customer list")
	}
}

func TestMigrationCutoverLock(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mig-cut-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	_, subID := createCustomerAndSubscription(t, apiKey, versionID)
	_ = subID

	// Create and start a migration (acquires cutover lock).
	status, body := apiReq(t, "POST", "/v1/migrations", apiKey, map[string]any{
		"source_system": "stripe",
	})
	jobID := body["id"].(string)

	apiReq(t, "POST", "/v1/migrations/"+jobID+"/records", apiKey, map[string]any{
		"records": []map[string]any{
			{
				"record_type": "customer",
				"external_id": "cus_cut_001",
				"source_data": map[string]any{"name": "Cut Corp", "email": "cut@example.com", "type": "business", "external_code": "ext_cut_001"},
			},
		},
	})
	apiReq(t, "POST", "/v1/migrations/"+jobID+"/validate", apiKey, nil)
	apiReq(t, "POST", "/v1/migrations/"+jobID+"/start", apiKey, nil)

	// New subscription creation should be blocked by cutover lock.
	status, body = apiReq(t, "POST", "/v1/subscriptions", apiKey, map[string]any{
		"external_id":          "sub_blocked",
		"customer_external_id": "ext_cut_001",
		"catalog_version_id":   versionID,
		"plan_code":            "starter",
	})
	if status != http.StatusConflict {
		t.Fatalf("create subscription during cutover: status %d, want 409, body %v", status, body)
	}
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "cutover_locked" {
		t.Fatalf("error code = %v, want cutover_locked", errObj["code"])
	}

	// Usage ingestion should also be blocked.
	now := time.Now().UTC().Format(time.RFC3339)
	status, _ = apiReq(t, "POST", "/v1/usage/ingest", apiKey, map[string]any{
		"transaction_id":       "txn_blocked",
		"customer_external_id": "ext_cut_001",
		"metric_code":          "api_calls",
		"timestamp":            now,
		"properties":           map[string]any{"quantity": 1},
	})
	if status != http.StatusConflict {
		t.Fatalf("ingest usage during cutover: status %d, want 409", status)
	}

	// Complete the migration to release the lock.
	apiReq(t, "POST", "/v1/migrations/"+jobID+"/complete", apiKey, nil)

	// Now subscription creation should work.
	status, body = apiReq(t, "POST", "/v1/subscriptions", apiKey, map[string]any{
		"external_id":          "sub_after",
		"customer_external_id": "ext_cut_001",
		"catalog_version_id":   versionID,
		"plan_code":            "starter",
	})
	if status != http.StatusCreated {
		t.Fatalf("create subscription after cutover: status %d, want 201, body %v", status, body)
	}
}

func TestMigrationRollback(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mig-rbk-"+uuid.NewString()[:8])

	// Create, validate, and start migration.
	status, body := apiReq(t, "POST", "/v1/migrations", apiKey, map[string]any{
		"source_system": "stripe",
	})
	jobID := body["id"].(string)

	apiReq(t, "POST", "/v1/migrations/"+jobID+"/records", apiKey, map[string]any{
		"records": []map[string]any{
			{
				"record_type": "customer",
				"external_id": "cus_rbk_001",
				"source_data": map[string]any{"name": "Rbk Corp", "email": "rbk@example.com", "type": "business", "external_code": "ext_rbk_001"},
			},
		},
	})
	apiReq(t, "POST", "/v1/migrations/"+jobID+"/validate", apiKey, nil)
	apiReq(t, "POST", "/v1/migrations/"+jobID+"/start", apiKey, nil)

	// Rollback.
	status, body = apiReq(t, "POST", "/v1/migrations/"+jobID+"/rollback", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("rollback: status %d, body %v", status, body)
	}
	if body["status"] != "rolled_back" {
		t.Fatalf("status = %v, want rolled_back", body["status"])
	}
	if body["cutover_locked"] != false {
		t.Fatal("cutover_locked should be false after rollback")
	}

	// Cannot rollback again.
	status, _ = apiReq(t, "POST", "/v1/migrations/"+jobID+"/rollback", apiKey, nil)
	if status != http.StatusConflict {
		t.Fatalf("double rollback: status %d, want 409", status)
	}
}

func TestMigrationCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "mig-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "mig-iso-b-"+uuid.NewString()[:8])

	// Provider A creates a migration job.
	status, body := apiReq(t, "POST", "/v1/migrations", keyA, map[string]any{
		"source_system": "stripe",
	})
	jobAID := body["id"].(string)

	// Provider B cannot see A's job.
	status, _ = apiReq(t, "GET", "/v1/migrations/"+jobAID, keyB, nil)
	if status != http.StatusNotFound {
		t.Fatalf("B get A's job: status %d, want 404", status)
	}

	// Provider B lists their own jobs (should be empty).
	status, body = apiReq(t, "GET", "/v1/migrations", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B list: status %d", status)
	}
	jobs := body["migration_jobs"].([]any)
	if len(jobs) != 0 {
		t.Fatalf("B: expected 0 jobs, got %d (RLS leak)", len(jobs))
	}
}

func TestMigrationValidationErrors(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mig-verr-"+uuid.NewString()[:8])

	// Missing source_system.
	status, _ := apiReq(t, "POST", "/v1/migrations", apiKey, map[string]any{
		"dry_run": false,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing source_system: status %d, want 400", status)
	}

	// Add records to non-existent job.
	status, _ = apiReq(t, "POST", "/v1/migrations/"+uuid.NewString()+"/records", apiKey, map[string]any{
		"records": []map[string]any{},
	})
	if status != http.StatusNotFound {
		t.Fatalf("non-existent job: status %d, want 404", status)
	}
}

func TestMigrationListJobs(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mig-lst-"+uuid.NewString()[:8])

	// Initially no jobs.
	status, body := apiReq(t, "GET", "/v1/migrations", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	jobs := body["migration_jobs"].([]any)
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}

	// Create two jobs.
	for i := 0; i < 2; i++ {
		apiReq(t, "POST", "/v1/migrations", apiKey, map[string]any{
			"source_system": "stripe",
		})
	}

	status, body = apiReq(t, "GET", "/v1/migrations", apiKey, nil)
	jobs = body["migration_jobs"].([]any)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestMigrationDuplicateRecordsSkipped(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mig-dup-"+uuid.NewString()[:8])

	status, body := apiReq(t, "POST", "/v1/migrations", apiKey, map[string]any{
		"source_system": "stripe",
	})
	jobID := body["id"].(string)

	// Add same record twice.
	rec := map[string]any{
		"records": []map[string]any{
			{
				"record_type": "customer",
				"external_id": "cus_dup",
				"source_data": map[string]any{"name": "Dup Corp", "email": "dup@example.com", "type": "business", "external_code": "ext_dup"},
			},
		},
	}
	apiReq(t, "POST", "/v1/migrations/"+jobID+"/records", apiKey, rec)
	status, body = apiReq(t, "POST", "/v1/migrations/"+jobID+"/records", apiKey, rec)
	if status != http.StatusOK {
		t.Fatalf("second add: status %d", status)
	}
	// Second add should report 0 added (ON CONFLICT DO NOTHING).
	if int(body["added"].(float64)) != 0 {
		t.Fatalf("added = %v, want 0 (duplicate skipped)", body["added"])
	}
}
