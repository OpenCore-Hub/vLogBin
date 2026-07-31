# vLogBin Platform Runbook

## P0 Runbooks

### R1: Regional Disaster Recovery (Cell Failover)

**Trigger**: Primary cell unavailable (database failure, region outage).

**Steps**:
1. Initiate failover via `POST /v1/operator/failovers`
2. Apply write fencing: `POST /v1/operator/failovers/{id}/fence`
3. Switch provider to standby cell: `POST /v1/operator/failovers/{id}/switch`
4. Verify application health on new cell
5. Complete failover (auto-replays unconfirmed Usage/Outbox): `POST /v1/operator/failovers/{id}/complete`
6. Monitor for 30 minutes before declaring recovery complete

**Rollback**: `POST /v1/operator/failovers/{id}/abort` (only before switch)

**Verification**:
- `GET /health` returns 200
- `GET /v1/operator/failovers/{id}` shows `completed`
- New usage ingestion succeeds
- No `cell_draining` errors

---

### R2: Provider Live Promotion (Test → Live)

**Prerequisites**:
- Provider signed Live agreement
- Risk review approved (Testing #15)
- PSP credentials configured
- Webhook endpoint validated

**Steps**:
1. Operator creates Live environment: `POST /v1/operator/providers/{id}/environments` with `kind=live`
2. Provider configures PSP credentials: `PUT /v1/psp-credentials`
3. Provider publishes Live catalog
4. Provider creates Live subscriptions
5. Verify invoice sync works: `POST /v1/invoices/sync`

**Verification**:
- Live environment exists with `kind=live`
- PSP credentials encrypted and stored
- Invoice sync returns invoices from Lago
- Audit log captures promotion event

---

### R3: Provider Offboarding (Data Export + Deletion)

**Trigger**: Provider requests offboarding.

**Steps**:
1. Export all data: `POST /v1/data-exports` with `export_type=full`
2. Verify export hash and record count
3. Deliver export to provider
4. Request deletion: `POST /v1/data-deletion` with `reason`
5. Verify deletion proof generated
6. Deliver deletion proof to provider
7. Revoke all credentials
8. Set provider cell to `inactive`

**Verification**:
- `GET /v1/deletion-proofs/{id}` shows proof with HMAC signature
- All credentials revoked
- Provider cell `inactive`
- Audit log captures full offboarding trail

---

### R4: Migration Cutover

**Trigger**: Provider migrating from external billing system.

**Steps**:
1. Create migration job: `POST /v1/migrations`
2. Add records: `POST /v1/migrations/{id}/records`
3. Validate (dry-run): `POST /v1/migrations/{id}/validate`
4. Review invalid records: `GET /v1/migrations/{id}/invalid-records`
5. Fix invalid records and re-validate
6. Start migration (acquires cutover lock): `POST /v1/migrations/{id}/start`
7. **Cutover lock active** — new subscriptions and usage blocked
8. Verify imported data
9. Complete migration (releases cutover lock): `POST /v1/migrations/{id}/complete`

**Rollback**: `POST /v1/migrations/{id}/rollback` (marks records as rolled_back)

**Verification**:
- `GET /v1/migrations/{id}` shows `completed`
- Cutover lock released
- New subscriptions and usage succeed
- Imported customers visible in `GET /v1/customers`

---

### R5: JIT Support Access

**Trigger**: Platform support engineer needs access to provider environment.

**Standard Access**:
1. Operator requests: `POST /v1/operator/providers/{id}/support-sessions` with `access_type=standard`
2. Provider approves: `POST /v1/support-sessions/{id}/approve`
3. Access granted for specified duration
4. Provider can revoke at any time: `POST /v1/support-sessions/{id}/revoke`

**Emergency Access (Two-Person Rule)**:
1. Operator requests: `POST /v1/operator/providers/{id}/support-sessions` with `access_type=emergency`
2. First operator approves: `POST /v1/operator/support-sessions/{id}/first-approve`
3. Second operator approves: `POST /v1/operator/support-sessions/{id}/second-approve`
4. Access granted (max 1 hour)
5. Auto-expires via background sweeper

**Verification**:
- `GET /v1/support-sessions/active` shows active sessions
- Audit log captures all access events
- Expired sessions auto-cleaned by sweeper

---

## P1 Runbooks

### R6: Webhook Delivery Failure

**Symptom**: Webhook deliveries failing or delayed.

**Steps**:
1. Check `GET /v1/webhook-deliveries` for failed deliveries
2. Verify provider endpoint is reachable
3. Check SSRF validation logs
4. If endpoint changed, provider updates: `POST /v1/webhooks` (new URL)
5. Dead-lettered deliveries after 3 retries
6. Provider can replay via event stream: `GET /v1/events`

### R7: Quota Exceeded

**Symptom**: `422 quota_exceeded` errors.

**Steps**:
1. Check current usage: `GET /v1/subscriptions/{id}/quota/usage`
2. If legitimate, increase limit: `PUT /v1/subscriptions/{id}/quota-limits/{key}`
3. If spike, check analytics: `GET /v1/analytics/anomalies`
4. Expired reservations auto-reclaimed by sweeper

### R8: Cell Migration

**Steps**:
1. Plan migration: `POST /v1/operator/cell-migrations`
2. Precheck: `POST /v1/operator/cell-migrations/{id}/precheck`
3. Execute: `POST /v1/operator/cell-migrations/{id}/execute`
4. Source cell stays `draining` until manually reactivated
5. Verify data on new cell
6. Reactivate source cell: `PATCH /v1/operator/cells/{from_cell_id}` with `status=active`

### R9: Budget Alert Triggered

**Symptom**: Budget alert status `warning` or `exceeded`.

**Steps**:
1. Check alert: `GET /v1/budget-alerts/{id}`
2. Review usage breakdown: `GET /v1/analytics/usage-breakdown`
3. If unexpected spike, check anomalies: `GET /v1/analytics/anomalies`
4. Adjust budget if needed: delete and recreate alert

### R10: SCIM Provisioning Failure

**Symptom**: SCIM user creation fails or enterprise IdP reports errors.

**Steps**:
1. Check SCIM user list: `GET /scim/v2/Users`
2. Verify API key has `scim:manage` scope
3. Check for duplicate external_id
4. Review audit log for SCIM operations
