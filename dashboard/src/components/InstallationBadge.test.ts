import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { InstallationBadge } from "./InstallationBadge";

describe("read-only installation class", () => {
  it.each(["normal", "pod"] as const)("shows locked %s without editing controls", kind => {
    const markup = renderToStaticMarkup(createElement(InstallationBadge, { installation: { kind } }));
    expect(markup).toContain(kind === "normal" ? "Normal" : "POD");
    expect(markup).toContain("Read-only");
    expect(markup).not.toMatch(/<(button|input|select)\b/);
    expect(markup).not.toContain("pod-active");
    expect(markup).not.toContain("pod-stby");
  });
  it("does not guess the class for older clients", () => {
    const markup = renderToStaticMarkup(createElement(InstallationBadge, {}));
    expect(markup).toContain("Not reported");
    expect(markup).not.toContain("Read-only");
  });
});
