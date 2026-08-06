import { VLogBinClient } from "./client.js";

export interface CreateCustomerInput {
  external_id: string;
  account_type: string;
  display_name: string;
}

export interface Customer {
  id: string;
  external_id: string;
  account_type: string;
  display_name: string;
}

export interface Subscription {
  id: string;
  external_id: string;
  customer_account_id: string;
  catalog_version_id: string;
  plan_id: string;
  status: string;
  started_at: string;
  terminated_at: string | null;
}

export interface IngestUsageInput {
  transaction_id: string;
  customer_external_id: string;
  metric_code: string;
  timestamp: string;
  properties: Record<string, unknown>;
}

export interface Event {
  id: string;
  event_type: string;
  aggregate_id: string;
  transaction_id: string;
  status: string;
  created_at: string;
}

export interface StreamResult {
  events: Event[];
  next_cursor: string | null;
  has_more: boolean;
}

export interface StreamEventsInput {
  cursor?: string;
  limit?: number;
  type?: string;
  aggregateType?: string;
}

export function createCustomer(
  client: VLogBinClient,
  input: CreateCustomerInput,
  idempotencyKey?: string,
): Promise<Customer> {
  return client.request<{ customer: Customer }>(
    "POST",
    "/customers",
    idempotencyKey ? { idempotencyKey } : {},
    input,
  ).then((out) => out.customer);
}

export function listSubscriptions(
  client: VLogBinClient,
): Promise<Subscription[]> {
  return client.request<{ subscriptions: Subscription[] }>(
    "GET",
    "/subscriptions",
  ).then((out) => out.subscriptions);
}

export function ingestUsage(
  client: VLogBinClient,
  input: IngestUsageInput,
  idempotencyKey?: string,
): Promise<{ status: string }> {
  return client.request(
    "POST",
    "/usage/ingest",
    idempotencyKey ? { idempotencyKey } : {},
    input,
  );
}

export function streamEvents(
  client: VLogBinClient,
  input: StreamEventsInput = {},
): Promise<StreamResult> {
  const query: Record<string, string> = { limit: String(input.limit ?? 100) };
  if (input.cursor) query.cursor = input.cursor;
  if (input.type) query.type = input.type;
  if (input.aggregateType) query.aggregate_type = input.aggregateType;
  return client.request("GET", "/events", { query });
}
