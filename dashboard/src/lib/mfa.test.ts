import { describe, expect, it } from "vitest";
import { isCompleteMfaCode, normalizeMfaInput } from "./mfa";

describe("normalizeMfaInput", () => {
  it("keeps only six digits for authenticator codes", () => {
    expect(normalizeMfaInput("12a 34-567", false)).toBe("123456");
  });

  it("uppercases and formats backup codes as XXXX-XXXX", () => {
    expect(normalizeMfaInput("abcd2345", true)).toBe("ABCD-2345");
    expect(normalizeMfaInput(" abcd - 2345 ", true)).toBe("ABCD-2345");
    expect(normalizeMfaInput("abc", true)).toBe("ABC");
    expect(normalizeMfaInput("abcde", true)).toBe("ABCD-E");
  });

  it("drops characters outside the base32 alphabet and caps at eight", () => {
    // 0, 1, 8, 9 are not base32.
    expect(normalizeMfaInput("a0b1c8d9efgh7xyz", true)).toBe("ABCD-EFGH");
  });
});

describe("isCompleteMfaCode", () => {
  it("accepts exactly six digits for authenticator codes", () => {
    expect(isCompleteMfaCode("123456", false)).toBe(true);
    expect(isCompleteMfaCode("12345", false)).toBe(false);
  });

  it("accepts only the full backup-code shape", () => {
    expect(isCompleteMfaCode("ABCD-2345", true)).toBe(true);
    expect(isCompleteMfaCode("ABCD2345", true)).toBe(false);
    expect(isCompleteMfaCode("ABCD-234", true)).toBe(false);
  });
});
