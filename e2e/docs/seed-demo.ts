/**
 * Seeds a running AllureDeck with a realistic, deterministic demo dataset for the
 * documentation screenshots (see `mise run docs:screenshots`).
 *
 *   cd e2e && ALLUREDECK_URL=... ALLUREDECK_API_URL=... npx tsx docs/seed-demo.ts
 *
 * Idempotent: the demo projects are deleted (children before parents) before re-seeding.
 * Build timestamps (`builds.created_at`) are not settable through the API; the mise task
 * backdates them with SQL after this script has run.
 */
import { createHash } from 'node:crypto'
import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'
import { chromium } from '@playwright/test'
import { AllureDeckClient } from '../fixtures/alluredeck-client'
import { USERNAME, PASSWORD } from '../fixtures/credentials'

const BASE_URL = process.env.ALLUREDECK_URL ?? 'http://localhost:7474'
const NOW = Date.now()
const HOUR = 3_600_000
const PIPELINE_RUNS = 12
const STANDALONE_BUILDS = 8
const GITLAB = 'https://gitlab.example.com'

// ── deterministic helpers ─────────────────────────────────────────────────────

type Rnd = () => number

function hash32(s: string): number {
  let h = 0x811c9dc5
  for (const c of s) {
    h ^= c.charCodeAt(0)
    h = Math.imul(h, 0x01000193)
  }
  return h >>> 0
}

function mulberry32(seed: number): Rnd {
  let a = seed
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const int = (r: Rnd, lo: number, hi: number) => lo + Math.floor(r() * (hi - lo + 1))
const pick = <T>(r: Rnd, xs: readonly T[]): T => xs[Math.floor(r() * xs.length)]
const hex = (r: Rnd, n: number) =>
  Array.from({ length: n }, () => '0123456789abcdef'[Math.floor(r() * 16)]).join('')
const uuid = (r: Rnd) => {
  const h = hex(r, 32)
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-4${h.slice(13, 16)}-a${h.slice(17, 20)}-${h.slice(20)}`
}
const md5 = (s: string) => createHash('md5').update(s).digest('hex')
const snake = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')

// ── failure messages (reused so defects cluster) ──────────────────────────────

interface Msg {
  message: string
  trace: string
}

const MSG = {
  // Regression introduced at run 9 (API side: order creation now answers 500).
  orderApi: {
    message: 'AssertionError: expected response status 201 but received 500',
    trace: [
      'AssertionError: expected response status 201 but received 500',
      '    at Object.<anonymous> (tests/checkout/orders.test.ts:48:23)',
      '    at OrdersClient.create (src/clients/orders.ts:73:19)',
      '    at processTicksAndRejections (node:internal/process/task_queues:95:5)',
    ].join('\n'),
  },
  // Same regression seen from the browser: the confirmation page never renders.
  orderWeb: {
    message: [
      'Error: expect(locator).toBeVisible() failed',
      '',
      "Locator: getByTestId('order-confirmation')",
      'Expected: visible',
      'Timeout: 5000ms',
      'Error: element(s) not found',
    ].join('\n'),
    trace: [
      "Error: expect(locator).toBeVisible() failed",
      "Locator: getByTestId('order-confirmation')",
      '    at /builds/webshop/shop/e2e/tests/checkout.spec.ts:57:46',
    ].join('\n'),
  },
  orderMobile: {
    message:
      'java.lang.AssertionError: Expected element id=order_confirmation_title to be displayed but was not found',
    trace: [
      'java.lang.AssertionError: Expected element id=order_confirmation_title to be displayed but was not found',
      '\tat com.webshop.mobile.CheckoutTest.completeCheckout(CheckoutTest.kt:88)',
      '\tat java.base/jdk.internal.reflect.DirectMethodHandleAccessor.invoke(DirectMethodHandleAccessor.java:103)',
    ].join('\n'),
  },
  // Flaky UI test.
  searchTimeout: {
    message:
      'TimeoutError: page.waitForResponse: Timeout 10000ms exceeded while waiting for event "response"',
    trace: [
      'TimeoutError: page.waitForResponse: Timeout 10000ms exceeded while waiting for event "response"',
      '    at /builds/webshop/shop/e2e/tests/search.spec.ts:31:16',
    ].join('\n'),
  },
  pullToRefresh: {
    message: 'TimeoutException: Timeout of 15000ms exceeded for element id=product_list_refresh',
    trace: [
      'TimeoutException: Timeout of 15000ms exceeded for element id=product_list_refresh',
      '\tat com.webshop.mobile.BrowseTest.pullToRefresh(BrowseTest.kt:41)',
    ].join('\n'),
  },
  inventorySync: {
    message: 'thrown: "Exceeded timeout of 5000 ms for a test."',
    trace: [
      'thrown: "Exceeded timeout of 5000 ms for a test."',
      '    at Object.<anonymous> (tests/inventory/sync.test.ts:22:5)',
    ].join('\n'),
  },
  // Environment outage: the whole run lost its database for a while.
  db: {
    message: 'connect ECONNREFUSED 10.0.3.14:5432',
    trace: [
      'Error: connect ECONNREFUSED 10.0.3.14:5432',
      '    at TCPConnectWrap.afterConnect [as oncomplete] (node:net:1555:16)',
    ].join('\n'),
  },
  refund: {
    message: 'AssertionError: expected refund amount 1049 to equal 1050 (half-up rounding)',
    trace: [
      'AssertionError: expected refund amount 1049 to equal 1050 (half-up rounding)',
      '\tat com.example.payments.RefundServiceTest.partialRefundRoundsHalfUp(RefundServiceTest.java:117)',
    ].join('\n'),
  },
  webhookRetry: {
    message: 'AssertionError: expected 3 delivery attempts but found 2',
    trace: [
      'AssertionError: expected 3 delivery attempts but found 2',
      '\tat com.example.payments.WebhookDispatcherTest.retriesWithBackoff(WebhookDispatcherTest.java:64)',
    ].join('\n'),
  },
  shardGreen: {
    message: 'TimeoutError: Timed out after 30s waiting for shard "products-v3" to turn green',
    trace: [
      'TimeoutError: Timed out after 30s waiting for shard "products-v3" to turn green',
      '\tat sync_test.go:73 (TestReindexWaitsForGreenShard)',
    ].join('\n'),
  },
} satisfies Record<string, Msg>

// ── catalogs ──────────────────────────────────────────────────────────────────

type Sev = 'blocker' | 'critical' | 'normal' | 'minor'
/** reg = regression, flaky/flaky2 = flipping tests, db = needs the database, skip = never runs */
type Tag = 'reg' | 'flaky' | 'flaky2' | 'db' | 'skip'
type Row = [ctx: string, name: string, sev?: Sev, tag?: Tag, skipReason?: string]
type Group = [suite: string, feature: string, rows: Row[]]

interface Rule {
  tag: Tag
  runs: number[]
  /** `retried` = failed first attempt, passed on retry (build stays green, test is flaky). */
  outcome: 'failed' | 'broken' | 'retried'
  msg: Msg
}

interface Spec {
  slug: string
  parent?: string
  kind: 'api' | 'web' | 'mobile'
  framework: string
  language: string
  parentSuite: string
  hosts: string[]
  /** [min, max] ms; most tests sit near the minimum. */
  durations: [number, number]
  groups: Group[]
  rules: Rule[]
  env: (run: number, sha: string) => Record<string, string>
}

const OWNERS = ['priya.raman', 'tom.becker', 'aiko.tanaka', 'marcus.webb']
const FROM = (first: number, last: number) => Array.from({ length: last - first + 1 }, (_, i) => first + i)

const webshopApi: Spec = {
  slug: 'webshop-api',
  parent: 'webshop',
  kind: 'api',
  framework: 'jest',
  language: 'typescript',
  parentSuite: 'REST API',
  hosts: ['ci-runner-01', 'ci-runner-02', 'ci-runner-03'],
  durations: [18, 900],
  groups: [
    ['Authentication', 'Auth', [
      ['POST /auth/login', 'returns a token for valid credentials', 'critical'],
      ['POST /auth/login', 'rejects an invalid password', 'critical'],
      ['POST /auth/refresh', 'rotates the refresh token'],
      ['DELETE /auth/logout', 'revokes the session'],
      ['POST /auth/password-reset', 'sends a reset email'],
    ]],
    ['Catalog', 'Catalog', [
      ['GET /products', 'lists products with pagination'],
      ['GET /products/{id}', 'returns product details'],
      ['GET /products', 'filters by category'],
      ['GET /products/search', 'ranks exact matches first'],
      ['GET /products', 'sorts by price ascending'],
      ['GET /categories', 'returns the category tree', 'minor'],
    ]],
    ['Cart', 'Cart', [
      ['POST /cart/items', 'adds an item'],
      ['PATCH /cart/items/{id}', 'updates the quantity'],
      ['DELETE /cart/items/{id}', 'removes an item'],
      ['POST /cart/coupons', 'applies a valid coupon'],
      ['POST /cart/coupons', 'rejects an expired coupon', 'minor'],
      ['GET /cart', 'recalculates totals with tax'],
    ]],
    ['Checkout', 'Checkout', [
      ['POST /orders', 'creates an order for a valid cart', 'blocker', 'reg'],
      ['POST /orders', 'rejects an empty cart'],
      ['POST /orders', 'creates an order with a coupon applied', 'critical', 'reg'],
      ['POST /payments/authorize', 'authorizes a valid card', 'blocker'],
      ['POST /payments/authorize', 'handles a declined card', 'critical'],
      ['POST /orders/{id}/confirm', 'sends the confirmation email'],
      ['POST /inventory/reserve', 'reserves stock for an order', 'critical'],
    ]],
    ['Orders', 'Orders', [
      ['GET /orders', "lists the customer's orders", 'normal', 'db'],
      ['GET /orders/{id}', 'returns order details', 'normal', 'db'],
      ['POST /orders/{id}/cancel', 'cancels an unshipped order', 'normal', 'db'],
      ['POST /orders/{id}/refund', 'refunds a delivered order', 'critical', 'db'],
      ['GET /orders/{id}/invoice', 'renders the invoice PDF', 'minor', 'skip', 'Skipped: PDF renderer is not available on CI runners'],
    ]],
    ['Customers', 'Customers', [
      ['GET /customers/me', 'returns the profile', 'normal', 'db'],
      ['PATCH /customers/me', 'updates the profile'],
      ['POST /customers/me/addresses', 'adds a shipping address'],
      ['DELETE /customers/me', 'schedules account deletion', 'minor', 'skip', 'Skipped: GDPR job runs only against staging'],
      ['GET /customers/me/wishlist', 'returns wishlist items', 'minor'],
    ]],
    ['Shipping', 'Shipping', [
      ['GET /shipping/rates', 'returns carrier rates'],
      ['POST /shipments', 'creates a shipment label'],
      ['GET /shipments/{id}/tracking', 'returns tracking events', 'minor'],
    ]],
    ['Inventory', 'Inventory', [
      ['GET /inventory/{sku}', 'returns stock levels'],
      ['POST /inventory/sync', 'syncs the warehouse feed', 'normal', 'flaky'],
      ['GET /inventory/low-stock', 'lists low stock alerts', 'minor'],
    ]],
  ],
  rules: [
    { tag: 'reg', runs: FROM(9, 12), outcome: 'failed', msg: MSG.orderApi },
    { tag: 'flaky', runs: [7], outcome: 'failed', msg: MSG.inventorySync },
    { tag: 'flaky', runs: [4], outcome: 'retried', msg: MSG.inventorySync },
    { tag: 'db', runs: [5], outcome: 'broken', msg: MSG.db },
  ],
  env: (run, sha) => ({
    'Base.URL': 'https://api.staging.webshop.example.com',
    'Node.Version': 'v22.14.0',
    Database: 'PostgreSQL 16.4',
    Commit: sha.slice(0, 8),
    Pipeline: `#${4810 + run}`,
    Runner: 'gitlab-runner-07',
  }),
}

const webshopWeb: Spec = {
  slug: 'webshop-web',
  parent: 'webshop',
  kind: 'web',
  framework: 'playwright',
  language: 'typescript',
  parentSuite: 'chromium',
  hosts: ['ci-runner-04', 'ci-runner-05'],
  durations: [1200, 9000],
  groups: [
    ['home.spec.ts', 'Home page', [
      ['/', 'shows featured products'],
      ['/', 'promotes the seasonal banner', 'minor'],
      ['/', 'navigates to a category from the mega menu'],
    ]],
    ['search.spec.ts', 'Search', [
      ['/search', 'autocompletes product names', 'normal', 'flaky'],
      ['/search', 'shows results for an exact SKU'],
      ['/search', 'shows an empty state for unknown queries', 'minor'],
      ['/search', 'keeps filters when sorting'],
    ]],
    ['product.spec.ts', 'Product page', [
      ['/products/trail-runner', 'switches images via the gallery'],
      ['/products/trail-runner', 'shows size availability'],
      ['/products/trail-runner', 'adds the product to the cart', 'critical'],
      ['/products/trail-runner', 'shows related products', 'minor'],
      ['/products/trail-runner', 'shows customer reviews', 'minor'],
    ]],
    ['cart.spec.ts', 'Cart', [
      ['/cart', 'updates the quantity from the mini cart'],
      ['/cart', 'removes an item'],
      ['/cart', 'persists the cart after reload', 'normal', 'flaky2'],
      ['/cart', 'applies a discount code'],
    ]],
    ['checkout.spec.ts', 'Checkout', [
      ['/checkout', 'guest can place an order', 'blocker', 'reg'],
      ['/checkout', 'signed-in customer can place an order', 'blocker', 'reg'],
      ['/checkout', 'validates the shipping address form'],
      ['/checkout', 'shows shipping options by country'],
      ['/checkout', 'pays with a saved card', 'critical', 'reg'],
    ]],
    ['account.spec.ts', 'Account', [
      ['/login', 'signs in with valid credentials', 'critical', 'db'],
      ['/account/orders', 'shows order history', 'normal', 'db'],
      ['/account/profile', 'edits the profile', 'normal', 'db'],
      ['/account/addresses', 'manages saved addresses'],
    ]],
    ['a11y.spec.ts', 'Accessibility', [
      ['/', 'home page has no critical axe violations'],
      ['/checkout', 'checkout form is keyboard navigable'],
      ['/', 'skip link moves focus to the main content', 'minor'],
    ]],
    ['responsive.spec.ts', 'Responsive layout', [
      ['/', 'collapses navigation into a drawer on mobile'],
      ['/checkout', 'keeps the cart summary sticky on tablet', 'minor', 'skip', 'Skipped: layout shifts on the tablet viewport (SHOP-1311)'],
    ]],
  ],
  rules: [
    { tag: 'reg', runs: FROM(9, 12), outcome: 'failed', msg: MSG.orderWeb },
    { tag: 'flaky', runs: [3, 7], outcome: 'failed', msg: MSG.searchTimeout },
    { tag: 'flaky', runs: [6], outcome: 'retried', msg: MSG.searchTimeout },
    { tag: 'flaky2', runs: [10], outcome: 'failed', msg: MSG.searchTimeout },
    { tag: 'flaky2', runs: [4, 8], outcome: 'retried', msg: MSG.searchTimeout },
    { tag: 'db', runs: [5], outcome: 'broken', msg: MSG.db },
  ],
  env: (run, sha) => ({
    Browser: 'Chromium',
    'Browser.Version': '137.0.7151.55',
    'Base.URL': 'https://staging.webshop.example.com',
    'Node.Version': 'v22.14.0',
    Playwright: '1.61.0',
    OS: 'Ubuntu 24.04',
    Commit: sha.slice(0, 8),
    Pipeline: `#${4810 + run}`,
  }),
}

const webshopMobile: Spec = {
  slug: 'webshop-mobile',
  parent: 'webshop',
  kind: 'mobile',
  framework: 'appium',
  language: 'kotlin',
  parentSuite: 'Android 15 / iOS 18',
  hosts: ['device-farm-01', 'device-farm-02'],
  durations: [3500, 22000],
  groups: [
    ['Onboarding', 'Onboarding', [
      ['welcome', 'shows the welcome carousel'],
      ['permissions', 'requests notification permission', 'minor'],
      ['welcome', 'lets the user skip onboarding', 'minor'],
    ]],
    ['Browse', 'Browse', [
      ['home', 'lists products on the home tab'],
      ['product', 'opens product details'],
      ['home', 'pull to refresh updates the product list', 'normal', 'flaky'],
      ['scanner', 'searches by barcode scan', 'minor', 'skip', 'Skipped: camera is unavailable on the emulator farm'],
    ]],
    ['Cart', 'Cart', [
      ['product', 'adds an item from the product screen'],
      ['cart', 'swipes to remove an item'],
      ['cart', 'syncs the cart across devices'],
    ]],
    ['Checkout', 'Checkout', [
      ['checkout', 'completes checkout with Google Pay or Apple Pay', 'blocker', 'reg'],
      ['checkout', 'completes checkout with a saved card', 'blocker', 'reg'],
      ['orders', 'shows the order status timeline'],
    ]],
    ['Account', 'Account', [
      ['login', 'signs in with biometrics', 'critical'],
      ['settings', 'updates push notification settings', 'minor'],
      ['settings', 'changes the app language', 'minor'],
    ]],
    ['Platform', 'Platform', [
      ['deeplink', 'handles deep links to products'],
      ['lifecycle', 'restores state after backgrounding'],
      ['network', 'shows an offline banner without network'],
      ['theme', 'renders dark mode correctly', 'minor', 'skip', 'Skipped: iOS simulator cannot toggle dark mode'],
    ]],
  ],
  rules: [
    { tag: 'reg', runs: FROM(9, 12), outcome: 'failed', msg: MSG.orderMobile },
    { tag: 'flaky', runs: [10], outcome: 'failed', msg: MSG.pullToRefresh },
    { tag: 'flaky', runs: [8], outcome: 'retried', msg: MSG.pullToRefresh },
  ],
  env: (run, sha) => ({
    Devices: 'Pixel 9 (Android 15), iPhone 16 (iOS 18.2)',
    'Appium.Version': '2.15.0',
    'App.Build': `2.${run}.0`,
    Commit: sha.slice(0, 8),
    Pipeline: `#${4810 + run}`,
  }),
}

const payments: Spec = {
  slug: 'payments-service',
  kind: 'api',
  framework: 'junit5',
  language: 'java',
  parentSuite: 'payments-service',
  hosts: ['ci-runner-11', 'ci-runner-12'],
  durations: [25, 700],
  groups: [
    ['Charges', 'Charges', [
      ['POST /charges', 'creates a charge for a valid card', 'blocker'],
      ['POST /charges', 'declines an expired card', 'critical'],
      ['POST /charges', 'handles a 3-D Secure challenge'],
      ['POST /charges', 'is idempotent for repeated keys', 'critical'],
      ['POST /charges', 'rejects unsupported currencies', 'minor'],
    ]],
    ['Refunds', 'Refunds', [
      ['POST /refunds', 'refunds a charge in full', 'critical'],
      ['POST /refunds', 'rounds a partial refund half-up', 'critical', 'reg'],
      ['POST /refunds', 'rejects refunds above the captured amount'],
      ['POST /refunds', 'is idempotent for repeated keys'],
      ['POST /refunds', 'updates the ledger balance'],
    ]],
    ['Webhooks', 'Webhooks', [
      ['POST /webhooks/deliver', 'signs outgoing payloads with HMAC'],
      ['POST /webhooks/deliver', 'retries failed deliveries with backoff', 'normal', 'flaky'],
      ['POST /webhooks/deliver', 'disables endpoints after repeated failures'],
      ['POST /webhooks/replay', 'replays a stored event', 'minor'],
      ['POST /webhooks/inbound', 'verifies provider signatures', 'critical'],
    ]],
    ['Fraud', 'Fraud checks', [
      ['POST /risk/score', 'flags velocity abuse'],
      ['POST /risk/score', 'blocks sanctioned countries', 'critical'],
      ['POST /risk/score', 'scores unknown devices'],
      ['POST /risk/score', 'allows trusted customers'],
      ['POST /risk/review', 'honours manual review decisions', 'minor', 'skip', 'Skipped: needs the sandbox reviewer queue'],
    ]],
    ['Ledger', 'Ledger', [
      ['ledger.post', 'posts double-entry records', 'critical', 'db'],
      ['ledger.reconcile', 'reconciles settlement files', 'normal', 'db'],
      ['ledger.convert', 'rounds multi-currency conversion'],
      ['ledger.close', 'closes the day atomically', 'normal', 'db'],
      ['ledger.export', 'exports statements as CSV', 'minor'],
    ]],
  ],
  rules: [
    { tag: 'reg', runs: FROM(6, 8), outcome: 'failed', msg: MSG.refund },
    { tag: 'flaky', runs: [3, 7], outcome: 'failed', msg: MSG.webhookRetry },
    { tag: 'flaky', runs: [5], outcome: 'retried', msg: MSG.webhookRetry },
    { tag: 'db', runs: [4], outcome: 'broken', msg: MSG.db },
  ],
  env: (run, sha) => ({
    'JDK.Version': '21.0.5',
    Database: 'PostgreSQL 16.4',
    Gateway: 'sandbox.payments.example.com',
    Commit: sha.slice(0, 8),
    Pipeline: `#${2310 + run}`,
  }),
}

const indexer: Spec = {
  slug: 'search-indexer',
  kind: 'api',
  framework: 'go test',
  language: 'go',
  parentSuite: 'search-indexer',
  hosts: ['ci-runner-21'],
  durations: [50, 2500],
  groups: [
    ['Indexing', 'Indexing', [
      ['index.Build', 'builds the product index from a snapshot'],
      ['index.Upsert', 'upserts changed documents only'],
      ['index.Delete', 'removes deleted products'],
      ['index.Mapping', 'applies analyzer settings per locale'],
      ['index.Alias', 'swaps the alias atomically', 'critical'],
    ]],
    ['Query', 'Query', [
      ['query.Parse', 'parses quoted phrases'],
      ['query.Filter', 'applies facet filters'],
      ['query.Suggest', 'returns prefix suggestions'],
      ['query.Typo', 'tolerates single-character typos'],
      ['query.Paginate', 'paginates with search_after', 'minor'],
    ]],
    ['Ranking', 'Ranking', [
      ['rank.Boost', 'boosts in-stock products'],
      ['rank.Synonyms', 'expands synonyms'],
      ['rank.Popularity', 'decays stale popularity signals', 'minor', 'skip', 'Skipped: needs the 30-day click log fixture'],
    ]],
    ['Sync', 'Sync', [
      ['sync.Reindex', 'reindex waits for the shard to be green', 'normal', 'flaky'],
      ['sync.Stream', 'applies catalog change events in order', 'critical'],
    ]],
  ],
  rules: [
    { tag: 'flaky', runs: [3, 6], outcome: 'failed', msg: MSG.shardGreen },
    { tag: 'flaky', runs: [5], outcome: 'retried', msg: MSG.shardGreen },
  ],
  env: (run, sha) => ({
    'Go.Version': '1.26.5',
    Elasticsearch: '8.15.3',
    Commit: sha.slice(0, 8),
    Pipeline: `#${770 + run}`,
  }),
}

// ── result generation ─────────────────────────────────────────────────────────

interface Assets {
  screenshot: Buffer
  trace: Buffer
}

interface AllureResult {
  uuid: string
  historyId: string
  testCaseId: string
  fullName: string
  name: string
  status: 'passed' | 'failed' | 'broken' | 'skipped'
  statusDetails?: { message: string; trace?: string; flaky?: boolean }
  stage: 'finished'
  start: number
  stop: number
  labels: Array<{ name: string; value: string }>
  steps: Array<Record<string, unknown>>
  attachments: Array<{ name: string; source: string; type: string }>
}

function fullNameOf(spec: Spec, suite: string, feature: string, name: string): string {
  if (spec.kind === 'web') return `${suite} › ${feature} › ${name}`
  if (spec.kind === 'mobile') return `com.webshop.mobile.${suite.replace(/\W/g, '')}Test#${snake(name)}`
  return `${spec.slug.replace(/-/g, '.')}.${suite.toLowerCase()}.${snake(name)}`
}

function stepNames(spec: Spec, ctx: string, name: string): string[] {
  if (spec.kind === 'web') {
    return [`page.goto("${ctx}")`, 'page.waitForLoadState("networkidle")', `Act: ${name}`, `Assert: ${name}`]
  }
  if (spec.kind === 'mobile') {
    return ['Launch the app', `Open the ${ctx} screen`, `Act: ${name}`, `Verify: ${name}`]
  }
  return [`Arrange: fixtures for ${ctx}`, `Act: ${ctx}`, 'Assert: response status', 'Assert: response body']
}

function attachmentsFor(
  spec: Spec,
  status: string,
  tag: Tag | undefined,
  rnd: Rnd,
  assets: Assets,
  files: Map<string, Buffer | string>,
): AllureResult['attachments'] {
  if (status !== 'failed' && status !== 'broken') return []
  const out: AllureResult['attachments'] = []
  const add = (name: string, ext: string, type: string, content: Buffer | string) => {
    const source = `${uuid(rnd)}-attachment.${ext}`
    files.set(source, content)
    out.push({ name, source, type })
  }
  if (spec.kind === 'api') {
    add('response-body', 'json', 'application/json', JSON.stringify({ error: 'internal_error', request_id: hex(rnd, 12) }, null, 2) + '\n')
    add('request-log', 'txt', 'text/plain', `${new Date(NOW).toISOString()} INFO  http  upstream responded in ${int(rnd, 40, 900)}ms\n`)
    return out
  }
  add('screenshot', 'png', 'image/png', assets.screenshot)
  if (spec.kind === 'web') {
    add('stderr', 'txt', 'text/plain', `[chromium] console.error: Failed to load resource: the server responded with a status of 500\n`)
    if (tag === 'reg') add('trace', 'zip', 'application/vnd.allure.playwright-trace', assets.trace)
  } else {
    add('logcat', 'txt', 'text/plain', `E/WebshopApp: CheckoutViewModel: submit failed (HTTP 500)\n`)
  }
  return out
}

/** Builds one results directory (result JSON, attachments, environment.properties) and returns its path. */
function buildResultsDir(spec: Spec, run: number, endMs: number, sha: string, assets: Assets): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), `demo-${spec.slug}-${run}-`))
  const rnd = mulberry32(hash32(`${spec.slug}:${run}`))
  const lanes = spec.hosts.flatMap((host) =>
    [1, 2].map((t) => ({ host, thread: `worker-${t}`, cursor: 0 })),
  )
  const files = new Map<string, Buffer | string>()
  const results: AllureResult[] = []

  for (const [suite, feature, rows] of spec.groups) {
    for (const [ctx, name, sev = 'normal', tag, skipReason] of rows) {
      const fullName = fullNameOf(spec, suite, feature, name)
      const rule = tag ? spec.rules.find((r) => r.tag === tag && r.runs.includes(run)) : undefined
      const baseRnd = mulberry32(hash32(`${spec.slug}|${fullName}`))
      const [lo, hi] = spec.durations
      const baseDur = lo + baseRnd() * baseRnd() * (hi - lo)

      const attempts: Array<'passed' | 'failed' | 'broken' | 'skipped'> =
        tag === 'skip' ? ['skipped']
        : !rule ? ['passed']
        : rule.outcome === 'retried' ? ['failed', 'passed']
        : [rule.outcome]

      for (const status of attempts) {
        const broken = status === 'broken'
        const failing = status === 'failed' || broken
        let dur = Math.round(baseDur * (0.85 + rnd() * 0.3))
        if (status === 'skipped') dur = 0
        else if (broken) dur = int(rnd, 40, 240)
        else if (failing) dur = Math.max(dur, spec.kind === 'web' ? 5200 : spec.kind === 'mobile' ? 15200 : dur)

        const lane = lanes.reduce((a, b) => (b.cursor < a.cursor ? b : a))
        const start = lane.cursor + int(rnd, 40, 400)
        const stop = start + dur
        lane.cursor = stop

        // 2–4 steps; the assertion step carries the failure, a broken test dies during arrange.
        const all = stepNames(spec, ctx, name)
        const count = int(rnd, 2, 4)
        const chosen = count === 4 ? all : count === 3 ? [all[0], all[1], all[3]] : [all[1], all[3]]
        const steps: AllureResult['steps'] = []
        if (status !== 'skipped') {
          const run1 = broken ? [chosen[0]] : chosen
          const slice = dur / run1.length
          run1.forEach((stepName, i) => {
            const last = i === run1.length - 1
            const stepStatus = last && failing ? status : 'passed'
            steps.push({
              name: stepName,
              status: stepStatus,
              ...(last && failing && rule ? { statusDetails: { message: rule.msg.message } } : {}),
              start: Math.round(start + i * slice),
              stop: Math.round(start + (i + 1) * slice),
              steps: [],
              attachments: [],
            })
          })
        }

        const owner = OWNERS[hash32(suite) % OWNERS.length]
        const flakyTest = tag === 'flaky' || tag === 'flaky2'
        results.push({
          uuid: uuid(rnd),
          historyId: md5(`${spec.slug}|${fullName}`),
          testCaseId: md5(fullName),
          fullName,
          name,
          status,
          ...(status === 'skipped'
            ? { statusDetails: { message: skipReason ?? 'Skipped' } }
            : failing && rule
              ? { statusDetails: { message: rule.msg.message, trace: rule.msg.trace, flaky: flakyTest } }
              : flakyTest && rule
                ? { statusDetails: { message: '', flaky: true } }
                : {}),
          stage: 'finished',
          start,
          stop,
          labels: [
            { name: 'parentSuite', value: spec.parentSuite },
            { name: 'suite', value: suite },
            { name: 'subSuite', value: feature },
            { name: 'feature', value: feature },
            { name: 'story', value: ctx },
            { name: 'severity', value: sev },
            { name: 'owner', value: owner },
            { name: 'tag', value: sev === 'blocker' || sev === 'critical' ? 'smoke' : 'regression' },
            { name: 'host', value: lane.host },
            { name: 'thread', value: lane.thread },
            { name: 'framework', value: spec.framework },
            { name: 'language', value: spec.language },
          ],
          steps,
          attachments: attachmentsFor(spec, status, tag, rnd, assets, files),
        })
      }
    }
  }

  // Shift so the run ends at endMs.
  const shift = endMs - Math.max(...results.map((r) => r.stop))
  const move = (r: AllureResult) => {
    r.start += shift
    r.stop += shift
    for (const s of r.steps) {
      s.start = (s.start as number) + shift
      s.stop = (s.stop as number) + shift
    }
  }
  results.forEach(move)

  for (const r of results) fs.writeFileSync(path.join(dir, `${r.uuid}-result.json`), JSON.stringify(r))
  for (const [source, content] of files) fs.writeFileSync(path.join(dir, source), content)
  const env = spec.env(run, sha)
  fs.writeFileSync(
    path.join(dir, 'environment.properties'),
    Object.entries(env).map(([k, v]) => `${k}=${v}`).join('\n') + '\n',
  )
  // Allure only groups failures into categories when the results ship a categories.json.
  fs.writeFileSync(path.join(dir, 'categories.json'), JSON.stringify(CATEGORIES))
  return dir
}

const CATEGORIES = [
  { name: 'Infrastructure problems', matchedStatuses: ['broken'], messageRegex: '(?s).*ECONNREFUSED.*' },
  { name: 'Timeouts', matchedStatuses: ['failed'], messageRegex: '(?s).*(waitForResponse|Timeout of|Exceeded timeout|Timed out).*' },
  { name: 'Product defects', matchedStatuses: ['failed'] },
  { name: 'Test defects', matchedStatuses: ['broken'] },
]

// ── assets: a failure screenshot and a real Playwright trace ──────────────────

async function recordAssets(): Promise<Assets> {
  const browser = await chromium.launch()
  try {
    const shot = await browser.newPage({ viewport: { width: 1280, height: 720 } })
    await shot.setContent(`<!doctype html><html><body style="margin:0;font-family:system-ui,sans-serif;background:#f6f7f9;color:#1c2330">
      <header style="background:#0f172a;color:#fff;padding:16px 32px;display:flex;justify-content:space-between"><b>WebShop</b><span>Cart (2)</span></header>
      <main style="max-width:880px;margin:40px auto;padding:0 24px">
        <h1 style="margin:0 0 16px">Checkout</h1>
        <div style="background:#fee2e2;border:1px solid #f87171;color:#7f1d1d;padding:14px 18px;border-radius:8px;margin-bottom:24px">
          Something went wrong while placing your order. Please try again.</div>
        <div style="display:grid;grid-template-columns:2fr 1fr;gap:24px">
          <section style="background:#fff;border-radius:10px;padding:24px;border:1px solid #e2e8f0"><h3 style="margin-top:0">Shipping address</h3>
            <p>Jane Doe<br>42 Example Street<br>Springfield, 12345</p><h3>Payment</h3><p>Visa ending in 4242</p></section>
          <aside style="background:#fff;border-radius:10px;padding:24px;border:1px solid #e2e8f0"><h3 style="margin-top:0">Order summary</h3>
            <p>Trail Runner x1 <b style="float:right">$129.00</b></p><p>Merino Socks x2 <b style="float:right">$32.00</b></p><hr>
            <p><b>Total</b> <b style="float:right">$161.00</b></p>
            <button style="width:100%;padding:12px;border:0;border-radius:8px;background:#2563eb;color:#fff;font-size:16px">Place order</button></aside>
        </div></main></body></html>`)
    const screenshot = await shot.screenshot()
    await shot.close()

    const context = await browser.newContext({ viewport: { width: 1280, height: 720 } })
    await context.tracing.start({ screenshots: true, snapshots: true })
    const page = await context.newPage()
    await page.goto(`${BASE_URL}/login`)
    await page.getByLabel('Username').fill('jane.doe@example.com')
    await page.getByLabel('Password').fill('correct-horse-battery')
    const tracePath = path.join(os.tmpdir(), 'demo-trace.zip')
    await context.tracing.stop({ path: tracePath })
    await context.close()
    return { screenshot, trace: fs.readFileSync(tracePath) }
  } finally {
    await browser.close()
  }
}

// ── upload helpers ────────────────────────────────────────────────────────────

async function ship(client: AllureDeckClient, slug: string, dir: string, query: Record<string, string>) {
  const before = await client.getLatestReportId(slug)
  const archive = `${dir}.tar.gz`
  AllureDeckClient.tarGzDirectory(dir, archive)
  await client.uploadAllureResults(slug, archive, query)
  await client.waitForNewReport(slug, before, 180_000)
  fs.rmSync(dir, { recursive: true, force: true })
  fs.rmSync(archive, { force: true })
}

const shaFor = (key: string) => hex(mulberry32(hash32(`sha:${key}`)), 40)

async function seedPipelineRuns(client: AllureDeckClient, assets: Assets) {
  const specs = [webshopApi, webshopWeb, webshopMobile]
  for (let run = 1; run <= PIPELINE_RUNS; run++) {
    const pipelineId = String(4810 + run)
    const sha = shaFor(`webshop:${run}`)
    const branch = run >= 6 && run <= 8 ? 'feature/checkout-v2' : 'main'
    const endMs = NOW - (PIPELINE_RUNS - run) * 6 * HOUR - 30_000
    console.log(`[seed] webshop pipeline #${pipelineId} (run ${run}/${PIPELINE_RUNS}, ${branch})`)
    await Promise.all(
      specs.map((spec) =>
        ship(client, spec.slug, buildResultsDir(spec, run, endMs, sha, assets), {
          execution_name: 'GitLab CI',
          execution_from: `${GITLAB}/webshop/shop/-/pipelines/${pipelineId}`,
          ci_branch: branch,
          ci_commit_sha: sha,
          ci_pipeline_id: pipelineId,
          ci_pipeline_url: `${GITLAB}/webshop/shop/-/pipelines/${pipelineId}`,
        }),
      ),
    )
  }
}

async function seedStandalone(client: AllureDeckClient, spec: Spec, assets: Assets) {
  for (let run = 1; run <= STANDALONE_BUILDS; run++) {
    const sha = shaFor(`${spec.slug}:${run}`)
    const endMs = NOW - (STANDALONE_BUILDS - run) * 6 * HOUR - 30_000
    console.log(`[seed] ${spec.slug} build ${run}/${STANDALONE_BUILDS}`)
    await ship(client, spec.slug, buildResultsDir(spec, run, endMs, sha, assets), {
      execution_name: 'GitLab CI',
      execution_from: `${GITLAB}/platform/${spec.slug}/-/pipelines/${spec.env(run, sha).Pipeline.slice(1)}`,
      ci_branch: 'main',
      ci_commit_sha: sha,
    })
  }
}

// ── post-upload data ──────────────────────────────────────────────────────────

interface ListResp<T> {
  data: T[]
}

async function seedMetadata(client: AllureDeckClient, ids: Map<string, number>) {
  const web = ids.get('webshop-web')!
  const pay = ids.get('payments-service')!

  // Known issues, matching real failing test names.
  await client.api('POST', `/projects/${web}/known-issues`, {
    test_name: 'autocompletes product names',
    pattern: 'page.waitForResponse',
    ticket_url: 'https://jira.example.com/browse/SHOP-1187',
    description: 'Suggestion request races the debounce on slow CI runners; fix is tracked with the search team.',
  })
  await client.api('POST', `/projects/${web}/known-issues`, {
    test_name: 'persists the cart after reload',
    pattern: 'page.waitForResponse',
    ticket_url: 'https://jira.example.com/browse/SHOP-1203',
    description: 'Cart sync request occasionally outlives the reload on the shared runners.',
  })
  await client.api('POST', `/projects/${pay}/known-issues`, {
    test_name: 'rounds a partial refund half-up',
    pattern: 'half-up rounding',
    ticket_url: 'https://jira.example.com/browse/PAY-902',
    description: 'Refund amounts are truncated instead of rounded when the captured amount has odd cents.',
  })

  // Triage a few of the auto-created defects.
  const defects = await client.api<ListResp<{ id: string; normalized_message: string }>>(
    'GET',
    `/projects/${web}/defects?per_page=100`,
  )
  const triage: Array<[needle: string, category: string, resolution: string]> = [
    ['order-confirmation', 'product_bug', 'open'],
    ['page.waitForResponse', 'test_bug', 'open'],
    ['ECONNREFUSED', 'infrastructure', 'muted'],
  ]
  for (const [needle, category, resolution] of triage) {
    const d = defects.data.find((x) => x.normalized_message.includes(needle))
    if (!d) throw new Error(`No defect matching "${needle}" in webshop-web`)
    await client.api('PATCH', `/projects/${web}/defects/${d.id}`, { category, resolution })
  }

  // Webhooks: never call /test, the targets are placeholders.
  await client.api('POST', `/projects/${web}/webhooks`, {
    name: 'Slack #webshop-ci',
    target_type: 'slack',
    url: 'https://hooks.slack.com/services/T0DEMO000/B0DEMO000/demoTokenNotARealSecret00',
    events: ['report_failed', 'regression_detected'],
  })
  await client.api('POST', `/projects/${web}/webhooks`, {
    name: 'Teams Release channel',
    target_type: 'teams',
    url: 'https://webshop.webhook.office.com/webhookb2/00000000-0000-0000-0000-000000000000@00000000-0000-0000-0000-000000000000/IncomingWebhook/00000000000000000000000000000000/00000000-0000-0000-0000-000000000000',
    events: ['report_completed', 'digest'],
  })

  // Users (skip when they already exist).
  const users = [
    { email: 'priya.raman@example.com', name: 'Priya Raman', role: 'admin' },
    { email: 'tom.becker@example.com', name: 'Tom Becker', role: 'editor' },
    { email: 'aiko.tanaka@example.com', name: 'Aiko Tanaka', role: 'editor' },
    { email: 'marcus.webb@example.com', name: 'Marcus Webb', role: 'viewer' },
  ]
  for (const u of users) {
    try {
      await client.api('POST', '/users', u)
    } catch (err) {
      if (!String(err).includes(' 409 ')) throw err
    }
  }

  // API keys: replace ours so reruns stay under the 5-per-user limit.
  const keyNames = ['GitLab CI uploader', 'webshop-web reporter']
  const keys = await client.api<ListResp<{ id: number; name: string }>>('GET', '/api-keys')
  for (const k of keys.data.filter((x) => keyNames.includes(x.name))) {
    await client.api('DELETE', `/api-keys/${k.id}`)
  }
  await client.api('POST', '/api-keys', {
    name: keyNames[0],
    expires_at: new Date(NOW + 90 * 24 * HOUR).toISOString(),
  })
  await client.api('POST', '/api-keys', {
    name: keyNames[1],
    expires_at: new Date(NOW + 30 * 24 * HOUR).toISOString(),
    project_ids: [web],
  })
}

// ── main ──────────────────────────────────────────────────────────────────────

async function main() {
  const client = new AllureDeckClient()
  await client.login(USERNAME, PASSWORD)

  console.log('[seed] removing previous demo projects')
  for (const slug of ['webshop-api', 'webshop-web', 'webshop-mobile', 'payments-service', 'search-indexer', 'webshop']) {
    await client.deleteProject(slug)
  }

  console.log('[seed] recording screenshot and Playwright trace assets')
  const assets = await recordAssets()

  const parent = await client.createProject('webshop')
  for (const spec of [webshopApi, webshopWeb, webshopMobile]) {
    await client.createProject(spec.slug, parent.project_id)
  }

  await Promise.all([
    seedPipelineRuns(client, assets),
    (async () => {
      await seedStandalone(client, payments, assets)
      await seedStandalone(client, indexer, assets)
    })(),
  ])

  const projects = await client.listProjects()
  const ids = new Map(projects.map((p) => [p.slug, p.project_id]))
  await seedMetadata(client, ids)

  console.log('\n[seed] done')
  for (const slug of ['webshop', 'webshop-api', 'webshop-web', 'webshop-mobile', 'payments-service', 'search-indexer']) {
    console.log(`  ${slug.padEnd(18)} project_id=${ids.get(slug)}`)
  }
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
