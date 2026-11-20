// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  createDesktopAuthCallbackUrl,
  parseDesktopAuthState,
} from "./desktop-handoff";

const VALID_STATE = "s".repeat(43);

describe("desktop auth handoff", () => {
  it("accepts only fixed-length base64url transaction states", () => {
    expect(parseDesktopAuthState(VALID_STATE)).toBe(VALID_STATE);
    expect(parseDesktopAuthState(null)).toBeNull();
    expect(parseDesktopAuthState("short")).toBeNull();
    expect(parseDesktopAuthState(`${"s".repeat(42)}+`)).toBeNull();
    expect(parseDesktopAuthState("s".repeat(44))).toBeNull();
  });

  it("returns the token and transaction state to Desktop", () => {
    expect(
      createDesktopAuthCallbackUrl("header.payload/signature", VALID_STATE),
    ).toBe(
      `multica://auth/callback?token=header.payload%2Fsignature&state=${VALID_STATE}`,
    );
    expect(() => createDesktopAuthCallbackUrl("token", "unbound")).toThrow(
      "invalid state",
    );
  });
});
