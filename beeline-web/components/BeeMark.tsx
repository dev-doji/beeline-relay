/** Beeline mark: a honey hexagon with a beeline (straight flight path). */
export function BeeMark({ size = 32, className }: { size?: number; className?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 40 40" className={className} aria-hidden="true">
      <path d="M20 2.5 35.2 11.25v17.5L20 37.5 4.8 28.75v-17.5Z" fill="#f5b324" />
      <path d="M11 24.5 20 15.5l4.2 4.2L30 13.9" fill="none" stroke="#1a1203" strokeWidth="3.2" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="30" cy="13.9" r="2.6" fill="#1a1203" />
    </svg>
  );
}
