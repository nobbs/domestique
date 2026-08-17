/**
 * The domestique chainring mark, drawn with `currentColor` so it takes the
 * surrounding text colour in both themes.
 *
 * The geometry is computed rather than hand-listed, matching how the brand
 * assets in `docs/brand` are generated: teeth and spider arms stay consistent
 * if the counts ever change.
 */

const TEETH = 24;
const SPIDER_ARMS = 5;
const TOOTH = "M59.6,23 L61.1,15.4 Q64,14 66.9,15.4 L68.4,23 Z";

function rotations(count: number): number[] {
  return Array.from({ length: count }, (_, index) => (index * 360) / count);
}

export interface LogoProps {
  size?: number;
  title?: string;
}

export function Logo({ size = 28, title = "domestique" }: LogoProps) {
  return (
    <svg
      viewBox="0 0 128 128"
      width={size}
      height={size}
      role="img"
      aria-label={title}
      focusable="false"
    >
      <title>{title}</title>
      <defs>
        <mask id="chainring-rim" maskUnits="userSpaceOnUse" x="0" y="0" width="128" height="128">
          <rect x="0" y="0" width="128" height="128" fill="#fff" />
          <circle cx="64" cy="64" r="29" fill="#000" />
        </mask>
      </defs>
      <g fill="currentColor">
        <g mask="url(#chainring-rim)">
          <circle cx="64" cy="64" r="44" />
          {rotations(TEETH).map((angle) => (
            <path key={angle} d={TOOTH} transform={`rotate(${angle} 64 64)`} />
          ))}
        </g>
        <circle cx="64" cy="64" r="9" />
        {rotations(SPIDER_ARMS).map((angle) => (
          <rect
            key={angle}
            x="59.25"
            y="30"
            width="9.5"
            height="34"
            transform={`rotate(${angle} 64 64)`}
          />
        ))}
      </g>
    </svg>
  );
}
