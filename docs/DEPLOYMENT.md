# vLogBin 平台运维安装与部署文档

## 1. 环境要求

### 1.1 生产环境

| 组件 | 最低版本 | 推荐配置 |
|---|---|---|
| Go | 1.25 | 1.25 |
| PostgreSQL | 16 | 16+ (4 vCPU, 8GB RAM, 100GB SSD) |
| Docker | 24.0 | 25.0+ |
| Kubernetes | 1.28 | 1.30+ |
| ZITADEL | 2.60 | 2.65+ |
| Lago | 1.0 | 1.0+ |

### 1.2 网络要求

- API 服务：入站 8080（或通过 Ingress 443）
- PostgreSQL：内部 5432
- ZITADEL：内部 8080
- Lago API：内部 3000
- 出站 HTTPS：Webhook 投递、ZITADEL Management API、Lago API

## 2. Docker Compose 部署

### 2.1 开发环境

```bash
# 启动开发环境
docker-compose -f docker-compose.dev.yml up -d

# 查看日志
docker-compose -f docker-compose.dev.yml logs -f api

# 停止
docker-compose -f docker-compose.dev.yml down
```

### 2.2 测试环境

```bash
# 启动测试环境（包含 ZITADEL + Lago + PostgreSQL）
docker-compose -f docker-compose.test.yml up -d

# 运行测试
go test ./internal/integration/ -count=1 -timeout 180s

# 停止并清理
docker-compose -f docker-compose.test.yml down -v
```

## 3. Kubernetes 部署

### 3.1 命名空间

```bash
kubectl create namespace vlogbin
kubectl create namespace vlogbin-zitadel
kubectl create namespace vlogbin-lago
```

### 3.2 配置

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: vlogbin-config
  namespace: vlogbin
data:
  PORT: "8080"
  BASE_DOMAIN: "api.vlogbin.com"
  DATABASE_URL: "postgres://platform_app@postgres:5432/vlogbin"
  ZITADEL_ISSUER: "https://auth.vlogbin.com"
  ZITADEL_MANAGEMENT_URL: "http://zitadel:8080/management/v1"
  LAGO_API_URL: "http://lago:3000"
  ENVIRONMENT: "production"
  SUPPORT_SWEEP_INTERVAL: "30s"
  QUOTA_SWEEP_INTERVAL: "15s"
  MIGRATION_SCHEDULE_INTERVAL: "5m"
```

```yaml
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: vlogbin-secrets
  namespace: vlogbin
type: Opaque
stringData:
  CRYPTO_KEY: "<32-byte-hex-key>"
  ZITADEL_PAT: "<zitadel-pat>"
  LAGO_API_KEY: "<lago-api-key>"
  OPERATOR_TOKEN: "<operator-static-token>"
```

### 3.3 部署

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vlogbin-api
  namespace: vlogbin
spec:
  replicas: 3
  selector:
    matchLabels:
      app: vlogbin-api
  template:
    metadata:
      labels:
        app: vlogbin-api
    spec:
      containers:
      - name: api
        image: ghcr.io/opencore-hub/vlogbin-api:latest
        ports:
        - containerPort: 8080
        envFrom:
        - configMapRef:
            name: vlogbin-config
        - secretRef:
            name: vlogbin-secrets
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
```

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: vlogbin-api
  namespace: vlogbin
spec:
  selector:
    app: vlogbin-api
  ports:
  - port: 8080
    targetPort: 8080
```

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: vlogbin-api
  namespace: vlogbin
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt
    nginx.ingress.kubernetes.io/rate-limit: "100"
spec:
  tls:
  - hosts: [api.vlogbin.com]
    secretName: vlogbin-tls
  rules:
  - host: api.vlogbin.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: vlogbin-api
            port:
              number: 8080
```

### 3.4 HPA 自动扩缩

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: vlogbin-api
  namespace: vlogbin
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: vlogbin-api
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

## 4. 数据库管理

### 4.1 迁移

```bash
# 运行迁移
goose -dir apps/api/db/migrations postgres "$DATABASE_URL" up

# 回滚最后一次迁移
goose -dir apps/api/db/migrations postgres "$DATABASE_URL" down

# 查看迁移状态
goose -dir apps/api/db/migrations postgres "$DATABASE_URL" status
```

### 4.2 备份与恢复（PITR）

```bash
# 创建基础备份
pg_basebackup -h postgres -D /backup/base -F c -z -P

# WAL 归档配置（postgresql.conf）
archive_mode = on
archive_command = 'aws s3 cp %p s3://vlogbin-wal-archive/%f'

# 恢复到指定时间点
pg_ctl -D /var/lib/postgresql/data stop
rm -rf /var/lib/postgresql/data/*
pg_basebackup -h backup-server -D /var/lib/postgresql/data -F c -z -P
echo "recovery_target_time = '2025-07-31 12:00:00'" > /var/lib/postgresql/data/recovery.signal
pg_ctl -D /var/lib/postgresql/data start
```

### 4.3 RLS 角色配置

```sql
-- 创建应用角色（RLS 强制）
CREATE ROLE platform_app WITH LOGIN PASSWORD 'secure-password';
GRANT USAGE ON SCHEMA public TO platform_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO platform_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO platform_app;

-- 启用所有表的 RLS
ALTER TABLE providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE providers FORCE ROW LEVEL SECURITY;
-- ... 对所有多租户表重复
```

## 5. 监控

### 5.1 健康检查

```bash
# 存活检查
curl http://localhost:8080/health

# 就绪检查
curl http://localhost:8080/ready
```

### 5.2 Prometheus 指标

```yaml
# servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: vlogbin-api
  namespace: vlogbin
spec:
  selector:
    matchLabels:
      app: vlogbin-api
  endpoints:
  - port: http
    path: /metrics
    interval: 30s
```

### 5.3 关键指标

| 指标 | 告警阈值 |
|---|---|
| HTTP 5xx 率 | > 1% 持续 5 分钟 |
| 请求延迟 P99 | > 500ms 持续 5 分钟 |
| Outbox pending 事件 | > 1000 持续 10 分钟 |
| 数据库连接池使用率 | > 80% |
| Webhook 投递失败率 | > 5% |

## 6. 生成加密密钥

```bash
# 生成 32 字节加密密钥（用于 AES-256-GCM）
openssl rand -hex 32

# 设置为环境变量
export CRYPTO_KEY="<generated-key>"
```

## 7. Docker 镜像构建

```bash
# 构建镜像
docker build -t vlogbin-api:latest apps/api/

# 多平台构建
docker buildx build --platform linux/amd64,linux/arm64 -t ghcr.io/opencore-hub/vlogbin-api:latest --push apps/api/
```
