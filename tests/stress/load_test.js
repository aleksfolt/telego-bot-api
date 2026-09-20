import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
const successfulRequests = new Counter('successful_requests');
const failedRequests = new Counter('failed_requests');
const errorRate = new Rate('error_rate');
const requestDuration = new Trend('custom_request_duration');

export const options = {
  stages: [
    { duration: '15s', target: 50 },  // Warm up: ramp-up to 50 concurrent virtual users
    { duration: '30s', target: 200 }, // Ramp-up to 200 VUs
    { duration: '1m',  target: 500 }, // Sustained heavy load at 500 VUs
    { duration: '30s', target: 800 }, // Spike test at 800 VUs
    { duration: '15s', target: 0 },   // Cool down back to 0
  ],
  thresholds: {
    'http_req_duration': ['p(95)<150', 'p(99)<300'], // 95% of requests must complete below 150ms
    'error_rate': ['rate<0.01'],                     // Less than 1% errors
  },
};

const BASE_URL = __ENV.SERVER_URL || 'http://127.0.0.1:8082';
const BOT_TOKEN = __ENV.BOT_TOKEN || '<BOT_TOKEN>';

const ENDPOINTS = [
  'getMe',
  'getMyCommands',
  'getWebhookInfo',
  'getMyName',
  'getMyDescription',
];

export default function () {
  const method = ENDPOINTS[Math.floor(Math.random() * ENDPOINTS.length)];
  const url = `${BASE_URL}/bot${BOT_TOKEN}/${method}`;

  const res = http.get(url, {
    headers: {
      'Accept': 'application/json',
      'User-Agent': 'k6-stress-tester',
    },
    timeout: '5s',
  });

  const isOk = check(res, {
    'status is 200': (r) => r.status === 200,
    'body has ok true': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.ok === true;
      } catch (e) {
        return false;
      }
    },
  });

  if (isOk) {
    successfulRequests.add(1);
    errorRate.add(0);
  } else {
    failedRequests.add(1);
    errorRate.add(1);
  }

  requestDuration.add(res.timings.duration);
}
