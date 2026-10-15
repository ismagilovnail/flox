import { describe, expect, it } from "vitest";

import { MIN_NAME_LENGTH, MIN_PASSWORD_LENGTH, emailField, nameField, newPasswordField } from "@/schemas/fields";

describe("shared field rules", () => {
  it("names: at least 2, at most the entity's max, trimmed when asked", () => {
    const name = nameField("too short", { max: 5 });
    expect(MIN_NAME_LENGTH).toBe(2);
    expect(name.safeParse("a").success).toBe(false);
    expect(name.safeParse("a").error?.issues[0].message).toBe("too short");
    expect(name.safeParse("ab").success).toBe(true);
    expect(name.safeParse("abcdef").success).toBe(false);
    expect(nameField("x", { max: 5 }).safeParse(" a ").success).toBe(true);
    expect(nameField("x", { max: 5, trim: true }).safeParse(" a ").success).toBe(false);
    expect(nameField("x", { max: 5, trim: true }).parse("  ab  ")).toBe("ab");
  });

  it("new passwords follow the API's minimum", () => {
    expect(MIN_PASSWORD_LENGTH).toBe(8);
    expect(newPasswordField("short").safeParse("1234567").success).toBe(false);
    expect(newPasswordField("short").safeParse("12345678").success).toBe(true);
  });

  it("emails", () => {
    expect(emailField("bad").safeParse("owner@flox.local").success).toBe(true);
    expect(emailField("bad").safeParse("owner").error?.issues[0].message).toBe("bad");
  });
});
