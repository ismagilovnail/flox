import { readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

/**
 * §31's layers: features build on the shared layers, never the other way.
 * A shared module importing a feature would tie every screen to it (and is
 * how the analytics types ended up in features/ while lib/api needed them).
 * Tests may cross — they check one layer against another.
 */
const SHARED_LAYERS = ["components", "hooks", "lib", "stores", "types", "schemas"];
const SOURCE = /\.(ts|tsx)$/;
const TEST = /\.test\.(ts|tsx)$/;
const FEATURE_IMPORT = /from\s+["']@\/features\/|import\(\s*["']@\/features\//;

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const full = path.join(dir, name);
    return statSync(full).isDirectory() ? files(full) : SOURCE.test(name) && !TEST.test(name) ? [full] : [];
  });
}

describe("frontend layers", () => {
  it("the shared layers do not import features", () => {
    const root = path.resolve(__dirname);
    const offenders = SHARED_LAYERS.flatMap((layer) => files(path.join(root, layer)))
      .filter((file) => FEATURE_IMPORT.test(readFileSync(file, "utf8")))
      .map((file) => path.relative(root, file));
    expect(offenders).toEqual([]);
  });
});
