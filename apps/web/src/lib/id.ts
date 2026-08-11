/** Base32-ish id generator shared by mock generators and stores. Accepts an
 * injectable PRNG so seeded generators stay deterministic. */
export function genId(rand: () => number = Math.random) {
  const chars = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
  let out = "";
  for (let i = 0; i < 12; i++) out += chars[Math.floor(rand() * chars.length)];
  return out;
}
