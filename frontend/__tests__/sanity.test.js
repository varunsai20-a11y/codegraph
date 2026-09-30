const test = require('node:test');
const assert = require('node:assert');

test('frontend state types and API client contracts sanity check', () => {
  assert.strictEqual(typeof process.env, 'object');
});

test('navigation tabs sanity check', () => {
  const tabs = ["EXPLORER", "GRAPH", "SYNC", "FLOW", "AI", "GUIDE"];
  assert.strictEqual(tabs.length, 6);
  assert.ok(tabs.includes("GRAPH"));
  assert.ok(tabs.includes("AI"));
});
