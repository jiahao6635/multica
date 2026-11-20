import { randomBytes } from "crypto";

const DESKTOP_AUTH_STATE_PATTERN = /^[A-Za-z0-9_-]{43}$/;
const DEFAULT_TRANSACTION_TTL_MS = 5 * 60 * 1000;

interface DesktopAuthFlowOptions {
  createState?: () => string;
  now?: () => number;
  transactionTtlMs?: number;
}

export class DesktopAuthFlow {
  private readonly pendingStates = new Map<string, number>();
  private readonly createState: () => string;
  private readonly now: () => number;
  private readonly transactionTtlMs: number;

  constructor(options: DesktopAuthFlowOptions = {}) {
    this.createState =
      options.createState ?? (() => randomBytes(32).toString("base64url"));
    this.now = options.now ?? Date.now;
    this.transactionTtlMs =
      options.transactionTtlMs ?? DEFAULT_TRANSACTION_TTL_MS;
  }

  beginLogin(appUrl: string): string {
    const now = this.now();
    this.removeExpired(now);

    const state = this.createState();
    if (!DESKTOP_AUTH_STATE_PATTERN.test(state)) {
      throw new Error("Desktop auth state generator returned an invalid state");
    }
    this.pendingStates.set(state, now + this.transactionTtlMs);

    const loginUrl = new URL("login", `${appUrl.replace(/\/+$/, "")}/`);
    loginUrl.searchParams.set("platform", "desktop");
    loginUrl.searchParams.set("desktop_state", state);
    return loginUrl.toString();
  }

  consumeCallback(url: URL): string | null {
    const token = url.searchParams.get("token");
    const state = url.searchParams.get("state");
    if (!token || !state || !DESKTOP_AUTH_STATE_PATTERN.test(state)) {
      return null;
    }

    const now = this.now();
    this.removeExpired(now);
    if (!this.pendingStates.has(state)) return null;

    // One completed login retires every parallel browser attempt. This keeps a
    // second tab from becoming a delayed account-switch callback.
    this.pendingStates.clear();
    return token;
  }

  private removeExpired(now: number): void {
    for (const [state, expiresAt] of this.pendingStates) {
      if (expiresAt <= now) this.pendingStates.delete(state);
    }
  }
}
