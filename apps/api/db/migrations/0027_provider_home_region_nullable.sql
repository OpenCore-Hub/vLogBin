-- +goose Up
-- Signup provisions a REGISTERED provider record for every workspace
-- (design baseline §2.1: workspace_id maps 1:1 to provider_id). A REGISTERED
-- provider has no home region yet — region and cell are assigned by the
-- operator when the provider is activated (REGISTERED → TEST_ACTIVE), not at
-- signup. The NOT NULL constraint is therefore lifted so the signup service
-- can create the record without guessing a region.
ALTER TABLE providers ALTER COLUMN home_region_id DROP NOT NULL;

-- +goose Down
-- Refuse to restore NOT NULL while unassigned REGISTERED providers exist,
-- rather than silently dropping rows or inventing regions for them.
ALTER TABLE providers ALTER COLUMN home_region_id SET NOT NULL;
