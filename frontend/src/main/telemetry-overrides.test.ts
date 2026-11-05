// @vitest-environment node
import { describe, it, expect } from "vitest";
import { telemetryOverrides } from "./telemetry-overrides";

describe("telemetryOverrides (fork build, D10)", () => {
	it("sends remote telemetry off and no Sentry DSN from a packaged build", () => {
		const env = telemetryOverrides({ isDev: false, appVersion: "1.2.3", env: {} });
		expect(env.AO_TELEMETRY_REMOTE).toBe("off");
		expect(env).not.toHaveProperty("AO_SENTRY_DSN");
		expect(env.AO_TELEMETRY_APP_VERSION).toBe("1.2.3");
	});

	it("keeps remote telemetry off in a dev build", () => {
		const env = telemetryOverrides({ isDev: true, appVersion: "1.2.3", env: {} });
		expect(env.AO_TELEMETRY_REMOTE).toBe("off");
		expect(env).not.toHaveProperty("AO_SENTRY_DSN");
	});

	it("lets an explicit env choice win", () => {
		const env = telemetryOverrides({
			isDev: false,
			appVersion: "1.2.3",
			env: { AO_TELEMETRY_REMOTE: "posthog", AO_SENTRY_DSN: "https://daemon.example/1" },
		});
		expect(env.AO_TELEMETRY_REMOTE).toBe("posthog");
		expect(env.AO_SENTRY_DSN).toBe("https://daemon.example/1");
	});
});
