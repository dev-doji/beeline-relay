/**
 * The Beeline mark: a honey hexagon with a notch cut into its left side and a straight line that fits
 * it exactly. Source of truth: brand/logo (GUIDELINES.md). Four cuts, picked automatically:
 *  - full, for sizes above 32 px (a band of negative space wraps the line);
 *  - small, for 32 px and below (thicker line, no band, so it does not close up);
 *  - onDark swaps the ink line for white. Use it on ink tiles, where an ink line would vanish.
 */
const HONEY = "#F5C451";
const INK = "#16162A";
const WHITE = "#FFFFFF";

const CUTS = {
  full: {
    shift: 12,
    cell: "M123 26.89A10 10 0 0 1 133 26.89L213.07 73.11A10 10 0 0 1 218.07 81.77L218.07 174.23A10 10 0 0 1 213.07 182.89L133 229.11A10 10 0 0 1 123 229.11L42.93 182.89A10 10 0 0 1 37.93 174.23L37.93 155A10 10 0 0 1 47.93 145L116 145A17 17 0 0 0 133 128L133 128A17 17 0 0 0 116 111L47.93 111A10 10 0 0 1 37.93 101L37.93 81.77A10 10 0 0 1 42.93 73.11Z",
    line: "M26 116H116A12 12 0 0 1 116 140H26A12 12 0 0 1 26 116Z",
  },
  fullDark: {
    shift: 12,
    cell: "M123 26.89A10 10 0 0 1 133 26.89L213.07 73.11A10 10 0 0 1 218.07 81.77L218.07 174.23A10 10 0 0 1 213.07 182.89L133 229.11A10 10 0 0 1 123 229.11L42.93 182.89A10 10 0 0 1 37.93 174.23L37.93 155A10 10 0 0 1 47.93 145L117 145A17 17 0 0 0 134 128L134 128A17 17 0 0 0 117 111L47.93 111A10 10 0 0 1 37.93 101L37.93 81.77A10 10 0 0 1 42.93 73.11Z",
    line: "M25 117H117A11 11 0 0 1 117 139H25A11 11 0 0 1 25 117Z",
  },
  small: {
    shift: 10,
    cell: "M123 12.89A10 10 0 0 1 133 12.89L225.19 66.11A10 10 0 0 1 230.19 74.77L230.19 181.23A10 10 0 0 1 225.19 189.89L133 243.11A10 10 0 0 1 123 243.11L30.81 189.89A10 10 0 0 1 25.81 181.23L25.81 158A10 10 0 0 1 35.81 148L108 148A20 20 0 0 0 128 128L128 128A20 20 0 0 0 108 108L35.81 108A10 10 0 0 1 25.81 98L25.81 74.77A10 10 0 0 1 30.81 66.11Z",
    line: "M24 108H108A20 20 0 0 1 108 148H24A20 20 0 0 1 24 108Z",
  },
  smallDark: {
    shift: 10,
    cell: "M123 12.89A10 10 0 0 1 133 12.89L225.19 66.11A10 10 0 0 1 230.19 74.77L230.19 181.23A10 10 0 0 1 225.19 189.89L133 243.11A10 10 0 0 1 123 243.11L30.81 189.89A10 10 0 0 1 25.81 181.23L25.81 156A10 10 0 0 1 35.81 146L110 146A18 18 0 0 0 128 128L128 128A18 18 0 0 0 110 110L35.81 110A10 10 0 0 1 25.81 100L25.81 74.77A10 10 0 0 1 30.81 66.11Z",
    line: "M22 110H110A18 18 0 0 1 110 146H22A18 18 0 0 1 22 110Z",
  },
} as const;

export function BeeMark({ size = 32, className, onDark = false }: { size?: number; className?: string; onDark?: boolean }) {
  const small = size <= 32;
  const cut = CUTS[small ? (onDark ? "smallDark" : "small") : onDark ? "fullDark" : "full"];
  return (
    <svg width={size} height={size} viewBox="0 0 256 256" className={className} aria-hidden="true">
      <g transform={`translate(${cut.shift} 0)`}>
        <path fill={HONEY} d={cut.cell} />
        <path fill={onDark ? WHITE : INK} d={cut.line} />
      </g>
    </svg>
  );
}
