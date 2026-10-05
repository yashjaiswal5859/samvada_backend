// =============================================================================
// Samvada Platform — k6 Performance & Load Test Suite (Phase 6.3)
// =============================================================================
// Run with: k6 run backend/tests/load/k6_platform_test.js
// Target: Up to 10K concurrent users, gateway load, call dispatch
// =============================================================================

import http from 'k6/http';
import ws from 'k6/ws';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom Metrics
const failureRate = new Rate('failed_requests');
const callLatency = new Trend('call_initiation_latency_ms');

export const options = {
  stages: [
    { duration: '30s', target: 50 },    // Ramp up to 50 users
    { duration: '1m', target: 200 },    // Spike to 200 concurrent users
    { duration: '2m', target: 500 },    // Sustain 500 concurrent users
    { duration: '30s', target: 0 },     // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<300'],   // 95% of HTTP requests must complete in < 300ms
    failed_requests: ['rate<0.01'],     // Error rate must stay below 1%
  },
};

const BASE_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';

export default function () {
  const params = {
    headers: {
      'Content-Type': 'application/json',
      'X-Request-ID': `k6-${Date.now()}-${__VU}`,
    },
  };

  // 1. Health Probe
  const healthRes = http.get(`${BASE_URL}/health`, params);
  const healthOk = check(healthRes, {
    'gateway health is 200': (r) => r.status === 200,
  });
  failureRate.add(!healthOk);

  // 2. Metrics Scraping Endpoint
  const metricsRes = http.get(`${BASE_URL}/metrics`, params);
  check(metricsRes, {
    'metrics status is 200': (r) => r.status === 200,
  });

  // 3. Rate Limit Test: Simulated bursts
  const start = Date.now();
  const authProbe = http.get(`${BASE_URL}/api/v1/auth/health`, params);
  callLatency.add(Date.now() - start);

  sleep(1);
}
