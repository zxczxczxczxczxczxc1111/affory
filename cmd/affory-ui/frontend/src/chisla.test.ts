import { expect, it } from "vitest";
import { slovoPosleChisla } from "./chisla";

// Grammar gets edge cases too; 111 is not three independent promises of singular.
it.each([
  [0, "правил"], [1, "правило"], [2, "правила"], [4, "правила"], [5, "правил"],
  [11, "правил"], [12, "правил"], [14, "правил"], [20, "правил"],
  [21, "правило"], [22, "правила"], [25, "правил"],
  [101, "правило"], [111, "правил"], [112, "правил"], [114, "правил"], [121, "правило"],
])("%i %s", (n, expected) => {
  expect(slovoPosleChisla(n, "правило", "правила", "правил")).toBe(expected);
});
