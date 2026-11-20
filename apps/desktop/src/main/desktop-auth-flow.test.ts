// @vitest-environment node
import { describe, expect, it } from "vitest";
import { DesktopAuthFlow } from "./desktop-auth-flow";

const STATE_A = "a".repeat(43);
const STATE_B = "b".repeat(43);

function callback(token: string, state?: string): URL {
  const url = new URL("multica://auth/callback");
  url.searchParams.set("token", token);
  if (state) url.searchParams.set("state", state);
  return url;
}

describe("DesktopAuthFlow", () => {
  it("binds browser login to a main-process transaction", () => {
    const flow = new DesktopAuthFlow({ createState: () => STATE_A });

    const loginUrl = new URL(flow.beginLogin("https://multica.ai/app"));

    expect(loginUrl.origin).toBe("https://multica.ai");
    expect(loginUrl.pathname).toBe("/app/login");
    expect(loginUrl.searchParams.get("platform")).toBe("desktop");
    expect(loginUrl.searchParams.get("desktop_state")).toBe(STATE_A);
  });

  it("rejects unsolicited and mismatched callbacks", () => {
    const flow = new DesktopAuthFlow({ createState: () => STATE_A });

    expect(flow.consumeCallback(callback("attacker-token", STATE_A))).toBeNull();

    flow.beginLogin("https://multica.ai");
    expect(flow.consumeCallback(callback("attacker-token"))).toBeNull();
    expect(flow.consumeCallback(callback("attacker-token", STATE_B))).toBeNull();
    expect(flow.consumeCallback(callback("legitimate-token", STATE_A))).toBe(
      "legitimate-token",
    );
  });

  it("consumes a matching callback exactly once", () => {
    const flow = new DesktopAuthFlow({ createState: () => STATE_A });
    flow.beginLogin("https://multica.ai");

    expect(flow.consumeCallback(callback("legitimate-token", STATE_A))).toBe(
      "legitimate-token",
    );
    expect(flow.consumeCallback(callback("replayed-token", STATE_A))).toBeNull();
  });

  it("retires parallel transactions after the first callback", () => {
    const states = [STATE_A, STATE_B];
    const flow = new DesktopAuthFlow({
      createState: () => states.shift() ?? STATE_A,
    });
    flow.beginLogin("https://multica.ai");
    flow.beginLogin("https://multica.ai");

    expect(flow.consumeCallback(callback("first-token", STATE_A))).toBe(
      "first-token",
    );
    expect(flow.consumeCallback(callback("late-token", STATE_B))).toBeNull();
  });

  it("rejects an expired transaction", () => {
    let now = 1_000;
    const flow = new DesktopAuthFlow({
      createState: () => STATE_A,
      now: () => now,
      transactionTtlMs: 5_000,
    });
    flow.beginLogin("https://multica.ai");

    now += 5_000;

    expect(flow.consumeCallback(callback("late-token", STATE_A))).toBeNull();
  });
});
