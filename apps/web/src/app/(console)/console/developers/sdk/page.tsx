import { requireAuth } from "@/lib/auth/rbac";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CodeBlock } from "@/components/ui/code-block";

export const dynamic = "force-dynamic";

const AUTH_CURL = `export VLOGBIN_API_URL="https://api.vlogbin.io"
export VLOGBIN_API_KEY="pk_test_..."

curl -X POST "$VLOGBIN_API_URL/v1/usage/ingest" \\
  -H "Authorization: Bearer $VLOGBIN_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "transaction_id": "tx_9f3c2a",
    "customer_external_id": "cust_acme",
    "metric_code": "api_calls",
    "timestamp": "2026-08-06T00:00:00Z",
    "properties": { "count": 1 }
  }'`;

const NODE_SNIPPET = `const res = await fetch(\`\${process.env.VLOGBIN_API_URL}/v1/usage/ingest\`, {
  method: "POST",
  headers: {
    Authorization: \`Bearer \${process.env.VLOGBIN_API_KEY}\`,
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    transaction_id: "tx_9f3c2a",
    customer_external_id: "cust_acme",
    metric_code: "api_calls",
    timestamp: new Date().toISOString(),
    properties: { count: 1 },
  }),
});`;

const PYTHON_SNIPPET = `import os
import requests

resp = requests.post(
    f"{os.environ['VLOGBIN_API_URL']}/v1/usage/ingest",
    headers={
        "Authorization": f"Bearer {os.environ['VLOGBIN_API_KEY']}",
        "Content-Type": "application/json",
    },
    json={
        "transaction_id": "tx_9f3c2a",
        "customer_external_id": "cust_acme",
        "metric_code": "api_calls",
        "timestamp": "2026-08-06T00:00:00Z",
        "properties": {"count": 1},
    },
)
print(resp.status_code)`;

export default async function SdkPage() {
  await requireAuth();

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">SDK</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          使用 API Key 上报用量、查询账单与订阅。密钥只显示一次，请从 API Keys 页面生成。
        </p>
      </header>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>cURL</CardTitle>
            <CardDescription>最轻量的接入方式，适用于脚本与 CI。</CardDescription>
          </CardHeader>
          <CardContent>
            <CodeBlock title="usage/ingest" language="bash" code={AUTH_CURL} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Node.js</CardTitle>
            <CardDescription>在服务端计量中间件中上报用量事件。</CardDescription>
          </CardHeader>
          <CardContent>
            <CodeBlock title="usage/ingest" language="ts" code={NODE_SNIPPET} />
          </CardContent>
        </Card>

        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Python</CardTitle>
            <CardDescription>批量任务或数据管道中按事务维度计量。</CardDescription>
          </CardHeader>
          <CardContent>
            <CodeBlock title="usage/ingest" language="python" code={PYTHON_SNIPPET} />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
