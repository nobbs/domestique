/**
 * The mark: a chainring, drawn as a chainring.
 *
 * Rim, teeth, spider and hub, all as filled geometry rather than as strokes
 * standing in for it. The rim is one even-odd path — outer circle, inner circle,
 * the ground between them — so the ring is a shape with a hole in it rather than
 * a line of some thickness, and the teeth sit on its edge as twenty-four
 * generated tabs at fifteen degrees apart.
 *
 * `currentColor` throughout: the mark is text, and takes the colour of the line
 * it sits in.
 */

/** One tooth, standing at the top of the rim before it is turned into place. */
const TOOTH = "M59.6,23 L61.1,15.4 Q64,14 66.9,15.4 L68.4,23 Z";

/** The rim: the ground between a 44 circle and a 29 one, punched out. */
const RIM =
  "M64,20 A44,44 0 1,0 64,108 A44,44 0 1,0 64,20 Z M64,35 A29,29 0 1,0 64,93 A29,29 0 1,0 64,35 Z";

const TEETH = Array.from({ length: 24 }, (_, index) => index * 15);
const ARMS = Array.from({ length: 5 }, (_, index) => index * 72);

export interface LogoProps {
  size?: number;
  title?: string;
  /** For the caller to set the colour the mark inherits, and nothing else. */
  className?: string;
}

export function Logo({ size = 28, title = "domestique", className }: LogoProps) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 128 128"
      fill="currentColor"
      role="img"
      aria-label={title}
      focusable="false"
    >
      <path d={RIM} fillRule="evenodd" />
      {TEETH.map((angle) => (
        <path key={angle} d={TOOTH} transform={`rotate(${angle} 64 64)`} />
      ))}
      {/* The spider: five arms from the hub out to the rim, and the hub. */}
      {ARMS.map((angle) => (
        <rect
          key={angle}
          x="59.25"
          y="30"
          width="9.5"
          height="34"
          transform={`rotate(${angle} 64 64)`}
        />
      ))}
      <circle cx="64" cy="64" r="9" />
    </svg>
  );
}
