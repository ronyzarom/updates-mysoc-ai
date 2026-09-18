import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { InstallationBadge } from "./InstallationBadge";

describe("read-only installation class", () => {
  it.each([["normal", "Normal"], ["pod", "POD"], ["pod-node", "POD node"], ["observer-unlinked", "POD Observer"]] as const)("shows locked %s without editing controls", (kind, label) => {
    const markup = renderToStaticMarkup(createElement(InstallationBadge, { installation: { kind } }));
    expect(markup).toContain(label);
    expect(markup).toContain("does not indicate linkage");
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
