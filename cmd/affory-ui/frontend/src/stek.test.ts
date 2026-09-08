import { describe, expect, it } from "vitest";

describe("стек тестов", () => {
  it("среда браузерная, а не node", () => {
    // jsdom must be enabled in vitest.config.ts. Without it every render from
    // @testing-library/react dies with "document is not defined", and the
    // failure reads like a screen bug when it is a config bug.
    expect(typeof document).toBe("object");
    expect(document.body).toBeTruthy();
  });
});
