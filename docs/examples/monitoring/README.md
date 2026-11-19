# Monitoring examples

Ready-made files for the Prometheus metrics at `/metrics` (see "Metrics" in `docs/operating.md`).

- `prometheus-scrape.yml`: a scrape job with the bearer token.
- `crowsnest-alerts.yml`: alert rules (crash loop, backups folder, damaged backup, offsite copy, low disk and memory).
- `grafana-dashboard.json`: a dashboard to import in Grafana (Dashboards, Import, pick your Prometheus).

The scrape job needs `server.metrics_token_env` set on the panel, with a token of at least 24 characters.
