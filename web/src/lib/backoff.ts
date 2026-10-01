/** Reconnect schedule from the spec: 1, 2, 4, 8, 16, then every 30 seconds. */
export const RETRY_DELAYS_S = [1, 2, 4, 8, 16, 30] as const;

/** Delay in seconds before retry number `attempt` (0-based). */
export function retryDelay(attempt: number): number {
  return RETRY_DELAYS_S[Math.min(Math.max(attempt, 0), RETRY_DELAYS_S.length - 1)];
}

export function bannerText(host: string, seconds: number): string {
  return `Lost connection to ${host}. Check that Tailscale is on for this device and for ${host}. Retrying in ${seconds} s.`;
}
