const DESKTOP_AUTH_STATE_PATTERN = /^[A-Za-z0-9_-]{43}$/;

export function parseDesktopAuthState(value: string | null): string | null {
  return value && DESKTOP_AUTH_STATE_PATTERN.test(value) ? value : null;
}

export function createDesktopAuthCallbackUrl(
  token: string,
  state: string,
): string {
  if (!parseDesktopAuthState(state)) {
    throw new Error("Cannot create a desktop callback with invalid state");
  }
  const params = new URLSearchParams({ token, state });
  return `multica://auth/callback?${params.toString()}`;
}
