import { requireAuth } from "@/lib/auth/rbac";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { CodeBlock } from "@/components/ui/code-block";

export const dynamic = "force-dynamic";

const EVENTS = [
  { type: "customer.created", aggregate: "customer_account", description: "客户创建" },
  { type: "subscription.created", aggregate: "subscription", description: "订阅创建" },
  { type: "subscription.terminated", aggregate: "subscription", description: "订阅终止" },
  { type: "usage.accepted", aggregate: "usage_event", description: "用量事件入库" },
  { type: "usage.reversed", aggregate: "usage_event", description: "用量事件撤销" },
  { type: "invoice.synced", aggregate: "invoice", description: "发票从计费引擎同步" },
  { type: "credential.created", aggregate: "credential", description: "API 密钥签发" },
  { type: "credential.revoked", aggregate: "credential", description: "API 密钥吊销" },
  { type: "webhook.endpoint_created", aggregate: "webhook", description: "Webhook 端点创建" },
  { type: "webhook.endpoint_deleted", aggregate: "webhook", description: "Webhook 端点删除" },
  { type: "plan.created", aggregate: "plan", description: "套餐创建" },
  { type: "plan.updated", aggregate: "plan", description: "套餐更新" },
  { type: "plan.published", aggregate: "plan", description: "目录版本发布" },
];

const PAYLOAD = `{
  "id": "evt_01J...",
  "event_type": "usage.accepted",
  "aggregate_type": "usage_event",
  "aggregate_id": "ue_01J...",
  "transaction_id": "tx_9f3c2a",
  "environment_kind": "test",
  "status": "published",
  "payload": {
    "customer_external_id": "cust_acme",
    "metric_code": "api_calls",
    "properties": { "count": 1 }
  },
  "created_at": "2026-08-06T00:00:00Z"
}`;

export default async function EventsSpecPage() {
  await requireAuth();

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">事件规范</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          平台事件是计费链路的可观测信号：可用于 Webhook 推送、审计与数据管道。
        </p>
      </header>

      <Card className="overflow-hidden">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold">事件目录</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-surface-2/70 text-left text-xs font-medium text-muted-foreground">
              <tr>
                <th className="px-4 py-3 font-medium">事件</th>
                <th className="px-4 py-3 font-medium">聚合</th>
                <th className="px-4 py-3 font-medium">说明</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {EVENTS.map((event) => (
                <tr key={event.type} className="transition-colors hover:bg-surface-2/60">
                  <td className="px-4 py-3">
                    <code className="font-mono text-xs text-foreground">
                      {event.type}
                    </code>
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant="neutral">{event.aggregate}</Badge>
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {event.description}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      <Card className="p-5">
        <h2 className="text-sm font-semibold">示例 payload</h2>
        <p className="mt-1 text-xs text-muted-foreground">
          事件流与 Webhook 投递使用同一事件结构。
        </p>
        <div className="mt-4">
          <CodeBlock title="usage.accepted" language="json" code={PAYLOAD} />
        </div>
      </Card>
    </div>
  );
}
