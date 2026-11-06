# Samvada Infrastructure as Code & Deployment

[![Kubernetes](https://img.shields.io/badge/Kubernetes-Helm_3-326CE5?style=flat&logo=kubernetes)](https://kubernetes.io/)
[![Prometheus](https://img.shields.io/badge/Prometheus-Monitoring-E6522C?style=flat&logo=prometheus)](https://prometheus.io/)
[![Grafana](https://img.shields.io/badge/Grafana-Dashboards-F46800?style=flat&logo=grafana)](https://grafana.com/)
[![NGINX](https://img.shields.io/badge/NGINX-API_Gateway-009639?style=flat&logo=nginx)](https://nginx.org/)

> **Production Infrastructure, Container Orchestration & Observability Suite**

---

## 🏛️ Directory Structure

```
infra/
├── clickhouse/                  # ClickHouse tables & materialized views initialization
├── gateway/                     # NGINX reverse proxy config & TLS certificate generator
├── k8s/helm/samvada/            # Production Kubernetes Helm chart (ABS + S2S with HPA)
├── minio/                       # S3 Object Storage bucket policies & CORS rules
├── observability/
│   ├── grafana/                 # Provisioned dashboards (Platform SLIs, Latencies)
│   ├── loki/                    # Loki & Promtail log shipping configuration
│   └── prometheus/              # Metrics scraping rules & alerting thresholds
└── postgres/                    # Database init scripts, schema migrations & seeds
```

---

## 📊 Pre-Provisioned Observability Dashboards

- **Grafana:** Located at `observability/grafana/dashboards/samvada_platform_slis.json`.
- **Metrics Scraped:**
  - HTTP request duration & error rates (RED metrics)
  - Speech-to-Speech audio turnaround latency (P50/P90/P99)
  - Kafka consumer lag per consumer group
  - PostgreSQL & ClickHouse active connection pools

For complete connection parameters, environment variables, and troubleshooting instructions, see the master [INFRASTRUCTURE.md](../INFRASTRUCTURE.md).

