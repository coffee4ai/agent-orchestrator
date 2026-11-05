import { DEFAULT_POSTHOG_HOST, DEFAULT_POSTHOG_PROJECT_KEY } from "../shared/posthog-config";
import { daemonSentryDsn } from "../shared/sentry-config";

export interface TelemetryOverridesInput {
	isDev: boolean;
	appVersion: string;
	env: NodeJS.ProcessEnv;
}

// Telemetry defaults stamped on the daemon env on every platform; explicit env
// always wins.
//
// Fork build (D10): remote telemetry is off by default in packaged builds too,
// and no Sentry DSN is sent unless the environment supplies one, so a daemon the
// desktop starts has both off. Local event recording stays on.
export function telemetryOverrides({ isDev, appVersion, env }: TelemetryOverridesInput): Record<string, string> {
	const sentryDsn = daemonSentryDsn(env.AO_SENTRY_DSN, isDev);
	return {
		AO_TELEMETRY_EVENTS: env.AO_TELEMETRY_EVENTS ?? "on",
		AO_TELEMETRY_REMOTE: env.AO_TELEMETRY_REMOTE ?? "off",
		AO_TELEMETRY_POSTHOG_KEY: env.AO_TELEMETRY_POSTHOG_KEY ?? DEFAULT_POSTHOG_PROJECT_KEY,
		AO_TELEMETRY_POSTHOG_HOST: env.AO_TELEMETRY_POSTHOG_HOST ?? DEFAULT_POSTHOG_HOST,
		// Daemon-side Sentry (5xx + panics with Go stacks): only an explicit env
		// DSN turns it on; a blank value leaves the daemon's Sentry a no-op.
		...(sentryDsn ? { AO_SENTRY_DSN: sentryDsn } : {}),
		// The daemon binary has no version of its own that release tooling sets,
		// so without this every daemon event lands unattributable to a release.
		AO_TELEMETRY_APP_VERSION: env.AO_TELEMETRY_APP_VERSION ?? appVersion,
		// Kill switch: forwarded so a noisy stream can be silenced by env on an
		// install that already exists, without shipping a new build.
		...(env.AO_TELEMETRY_DISABLED_EVENTS ? { AO_TELEMETRY_DISABLED_EVENTS: env.AO_TELEMETRY_DISABLED_EVENTS } : {}),
	};
}
