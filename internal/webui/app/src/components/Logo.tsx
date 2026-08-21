/**
 * The mark: a chainring, drawn as three circles.
 *
 * It is rendered at 26 px beside the wordmark and nowhere larger, so the teeth
 * were never resolved as teeth — twenty-four generated tooth paths came out as
 * a rough edge, which is exactly what a dashed stroke gives for three lines of
 * geometry. `pathLength` normalises the circle to 120 units so the dash pattern
 * counts in teeth rather than in whatever the radius happens to make.
 *
 * `currentColor` throughout: the mark is text, and takes the colour of the line
 * it sits in.
 */

export interface LogoProps {
  size?: number;
  title?: string;
}

export function Logo({ size = 28, title = "domestique" }: LogoProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 128 128"
      role="img"
      aria-label={title}
      focusable="false"
    >
      {/* The toothed rim. 120 normalised units at 3-on-2-off make 24 teeth. */}
      <circle
        cx="64"
        cy="64"
        r="52"
        fill="none"
        stroke="currentColor"
        strokeWidth="9"
        pathLength={120}
        strokeDasharray="3 2"
      />
      {/* The spider, as the ring it reads as at this size. */}
      <circle cx="64" cy="64" r="31" fill="none" stroke="currentColor" strokeWidth="7" />
      <circle cx="64" cy="64" r="10" fill="currentColor" />
    </svg>
  );
}
