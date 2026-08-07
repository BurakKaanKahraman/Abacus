/**
 * Load profile for the calculator API.
 *
 * Run against a stack that is already up:
 *
 *   docker compose up -d --wait
 *   k6 run tests/stress/k6_script.js
 *
 * Two things are being measured, and they pull in opposite directions:
 *
 *  1. How fast the expression engine answers under concurrency.
 *  2. That the rate limiter actually throttles, which by design means most
 *     virtual users receive 429 rather than 200.
 *
 * The limiter counts per client IP, and a load generator is a single IP, so a
 * run at 500 VUs against the production limit measures the limiter, not the
 * engine: almost every request is rejected before any arithmetic happens.
 *
 * The `latency` scenario therefore needs the backend started with the limit
 * raised, which is a property of the stack rather than of this script:
 *
 *   RATE_LIMIT_PER_MINUTE=6000000 RATE_LIMIT_BURST=100000 \
 *     docker compose up -d backend --wait
 *   k6 run tests/stress/k6_script.js
 *   docker compose up -d backend --wait        # restore the real limit
 *
 * Forgetting that step used to produce a run that reported "all thresholds
 * met" while having evaluated a few hundred of several hundred thousand
 * requests, so the scenario now fails if most of its traffic was throttled.
 *
 * The `limiter` scenario is the opposite: it keeps the production setting and
 * asserts that the service sheds load instead of degrading.
 */

import { check, sleep } from 'k6';
import http from 'k6/http';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080/api/v1';
const SCENARIO = __ENV.SCENARIO || 'latency';

// k6 counts every 4xx as a failed request. Being throttled is the limiter
// working, not the service failing, so 429 is declared expected: that keeps
// http_req_failed meaningful as "something actually went wrong", and a 400 or
// a 500 still trips it.
http.setResponseCallback(http.expectedStatuses(200, 429));

/** Share of requests that were throttled, tracked separately from failures. */
const throttled = new Rate('throttled_requests');
/** Latency of requests the server actually evaluated. */
const evaluationDuration = new Trend('evaluation_duration', true);

/**
 * Expressions of increasing cost. The heaviest is far beyond anything a
 * keypad produces, which is the point: it bounds the worst case.
 */
const EXPRESSIONS = [
  '1 + 1',
  '10 + 20 * 3',
  '10 + 20 * 3 - 15 / (5 - 2)',
  '(10 + sqrt(16)) * 2^3',
  '-10 + sqrt(16) * 2 - 100 % 7',
  '((((1 + 2) * 3) - 4) / 5) ^ 2 + sqrt(144) * 2 ^ 3 - 100 % 7',
  '1 + 2 * 3 - 4 / 5 + 6 * 7 - 8 / 9 + sqrt(100) * (11 - 12) + 13 ^ 2 - 14 % 5',
];

const scenarios = {
  /**
   * Ramps to 500 concurrent users to measure the engine under pressure.
   * Meaningful only when the limiter is raised for the run; see the header.
   */
  latency: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '20s', target: 100 },
      { duration: '30s', target: 500 },
      { duration: '40s', target: 500 },
      { duration: '20s', target: 0 },
    ],
    gracefulRampDown: '10s',
    exec: 'calculate',
  },

  /**
   * A steady burst against the production rate limit, asserting that the
   * service sheds load instead of degrading.
   */
  limiter: {
    executor: 'constant-arrival-rate',
    rate: 50,
    timeUnit: '1s',
    duration: '30s',
    preAllocatedVUs: 50,
    maxVUs: 200,
    exec: 'calculate',
  },

  /** A single user, for a clean baseline unaffected by queueing. */
  baseline: {
    executor: 'constant-vus',
    vus: 1,
    duration: '30s',
    exec: 'calculate',
  },
};

/**
 * Thresholds every scenario must meet.
 *
 * The latency percentiles deliberately exclude throttled requests: those
 * return before any evaluation and would flatter the number they are supposed
 * to police.
 */
const commonThresholds = {
  // Sub-10ms at the 95th percentile for requests the server evaluated.
  evaluation_duration: ['p(95)<10', 'p(99)<25'],

  // A request must never fail for a reason other than throttling: no
  // connection resets, no 5xx, no timeouts.
  checks: ['rate>0.99'],
  http_req_failed: ['rate<0.01'],
};

/**
 * Per-scenario expectations about throttling. These turn a misconfigured run
 * into a failure rather than a green report that measured nothing.
 */
const scenarioThresholds = {
  // Almost everything must be evaluated, or the limit was not raised and the
  // percentiles above describe a few stray requests.
  latency: { throttled_requests: ['rate<0.05'] },
  // Throttling is the assertion here, not a side effect.
  limiter: { throttled_requests: ['rate>0.5'] },
  baseline: { throttled_requests: ['rate<0.05'] },
};

export const options = {
  scenarios: { [SCENARIO]: scenarios[SCENARIO] },

  // p(99) is not computed by default, and the summary below reports it.
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],

  thresholds: { ...commonThresholds, ...scenarioThresholds[SCENARIO] },

  noConnectionReuse: false,
  discardResponseBodies: false,
};

export function calculate() {
  const expression = EXPRESSIONS[Math.floor(Math.random() * EXPRESSIONS.length)];

  const response = http.post(`${BASE_URL}/calculate`, JSON.stringify({ expression }), {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'POST /calculate' },
  });

  const isThrottled = response.status === 429;
  throttled.add(isThrottled);

  if (!isThrottled) {
    evaluationDuration.add(response.timings.duration);
  }

  check(response, {
    'status is 200 or 429': (r) => r.status === 200 || r.status === 429,
    'evaluated requests return a numeric result': (r) => {
      if (r.status !== 200) return true;
      const body = r.json();
      return typeof body.result === 'number' && Number.isFinite(body.result);
    },
    'throttled requests are RFC 7807 documents': (r) => {
      if (r.status !== 429) return true;
      return r.json().code === 'ERR_RATE_LIMIT_EXCEEDED' && r.headers['Retry-After'] !== undefined;
    },
    'precedence holds under load': (r) => {
      if (r.status !== 200) return true;
      const body = r.json();
      // The one expression whose value is asserted, to prove the engine is not
      // returning stale or truncated answers when saturated.
      return body.expression !== '10 + 20 × 3' || body.result === 70;
    },
  });

  sleep(0.1);
}

/**
 * A compact summary.
 *
 * Threshold and check outcomes are reported explicitly: overriding
 * handleSummary replaces k6's own report, and a run that silently hides
 * whether it met its thresholds is worse than no report at all.
 */
export function handleSummary(data) {
  const value = (name, stat) => data.metrics[name]?.values?.[stat];
  const number = (name, stat, digits = 2) => value(name, stat)?.toFixed(digits) ?? 'n/a';

  const checks = data.metrics.checks?.values;
  const checksPassed = checks ? `${(checks.rate * 100).toFixed(2)}% (${checks.passes}/${checks.passes + checks.fails})` : 'n/a';

  const thresholdLines = [];
  let failed = false;
  for (const [metricName, metric] of Object.entries(data.metrics)) {
    for (const [expression, outcome] of Object.entries(metric.thresholds ?? {})) {
      const ok = outcome.ok !== false;
      if (!ok) failed = true;
      thresholdLines.push(`  ${ok ? 'PASS' : 'FAIL'}  ${metricName}: ${expression}`);
    }
  }

  const lines = [
    '',
    '─'.repeat(58),
    ` Scenario            ${SCENARIO}`,
    ` Requests            ${value('http_reqs', 'count') ?? 0}`,
    ` Checks passed       ${checksPassed}`,
    ` Throttled (429)     ${((value('throttled_requests', 'rate') ?? 0) * 100).toFixed(1)}%`,
    '',
    ` Evaluated p(95)     ${number('evaluation_duration', 'p(95)')} ms`,
    ` Evaluated p(99)     ${number('evaluation_duration', 'p(99)')} ms`,
    ` Evaluated max       ${number('evaluation_duration', 'max')} ms`,
    '',
    ' Thresholds',
    ...thresholdLines,
    '─'.repeat(58),
    failed ? ' RESULT: thresholds not met' : ' RESULT: all thresholds met',
    '',
  ];

  return {
    stdout: lines.join('\n'),
    'summary.json': JSON.stringify(data, null, 2),
  };
}
