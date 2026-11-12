/** Decorative hex lattice used on dark art tiles (same look as the landing page). */
export function Honeycomb() {
  const hexes = [];
  for (let r = 0; r < 6; r++) {
    for (let c = 0; c < 6; c++) {
      const x = c * 30 + (r % 2 ? 15 : 0);
      const y = r * 26;
      const filled = (r * 7 + c * 3) % 5 === 0;
      hexes.push(
        <path
          key={`${r}-${c}`}
          d={`M${x + 15} ${y} l13 7.5v15l-13 7.5-13-7.5v-15z`}
          className={filled ? "hx on" : "hx"}
          style={{ animationDelay: `${(r + c) * 120}ms` }}
        />,
      );
    }
  }
  return (
    <svg viewBox="-4 -4 200 170" aria-hidden="true" className="comb">
      {hexes}
    </svg>
  );
}
