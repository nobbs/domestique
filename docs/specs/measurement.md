# Domestique measurement specification

**Status:** accepted

This specification is subordinate to [the service contract](service.md). It
holds every formula this service computes over ground and time, where each
formula comes from, and which package function applies it. A change to a
constant or a method here is a behaviour change, and it lands with the code
that makes it.

## Spherical distance

**Definition.** The great-circle distance between two points on a sphere.

**Formula.** In symbols:

~~~text
a      = sin²(Δlat/2) + cos(lat1)·cos(lat2)·sin²(Δlon/2)
d      = R · 2 · atan2(sqrt(a), sqrt(1 - a))
~~~

d is in metres; lat and lon are in radians.

**Constants.** R = `measure.EarthRadiusMetres` = 6 371 000 m
(`internal/measure/geo.go`).

**Source.** Sinnott 1984 (haversine formula).

**Applied by.** `internal/measure/geo.go` `HaversineMetres`. Callers reach it
as `measure.HaversineMetres` via `route.Point.Coordinate()`:
`route` itself (route length), `fit`, `wahoo`, `surface`, `ridemodel`,
`demo`. `route.CumulativeMetres` is the running sum over a geometry, built on
`measure.CumulativeMetres`. `internal/webui/app/src/lib/profile.ts`
`haversineMetres` is the browser's own copy on the same radius
(`EARTH_RADIUS_METRES`), kept in step under
[implementation-architecture.md](implementation-architecture.md)'s
"interaction pulls work forwards" rule: route-only arithmetic run on every
hover or drag frame is implemented in the browser as well as in Go.

**Status.** Validated: this is the standard formula, and both
implementations agree to the metre by construction.

## Profiles

**Definition.** Altitude as a function of cumulative distance, resampled to
an even grid and smoothed with a centred moving median.

**Formula.** In symbols:

~~~text
resample: altitude at every 25 m of cumulative distance, linearly
          interpolated between the two source points either side
filter:   for sample i, radius = int(window / interval / 2)
          window  = elevations[i - radius .. i + radius], clamped to the ends
          median  = sorted(window)[len(window) / 2]
~~~

Integer division on an even-length window keeps the upper of the two middle
values, because that is what a single sorted index gives without a second
case for the even length.

**Constants.** Sample interval = 25 m, median window = 100 m
(`internal/elevation/normalizer.go` `sampleIntervalMetres`,
`medianWindowMetres`).

**Source.** This service's own rule. A median rather than a mean because an
isolated spike in one sample does not drag several neighbours' filtered
values with it; a mean would.

**Applied by.** `elevation.Normalizer.Process` through
`measure.Profile.Resample`, `MedianFiltered` and `AltitudeAt`
(`internal/measure/profile.go`), before device export.

**Status.** Validated: this is the exported profile riders load onto a
device today.

## Gradient

**Definition.** Signed rise over run, measured back over the shortest span
of at least W metres, rather than between adjacent samples where altitude
error dominates.

**Formula.** In symbols:

~~~text
grade% = (elevation[i] - elevation[trailing])
         / (distance[i] - distance[trailing]) · 100
~~~

where `trailing` is walked forward only as far as keeps
`distance[i] - distance[trailing] >= W`.

Distance, not time, is the axis: a window has to mean the same span of
ground whether the rider was climbing at walking pace or descending at
speed, and a time window would stretch and shrink with neither.

**Window derivation.** A barometric altimeter reports altitude to one
quantisation step. Over a window shorter than that step's own worth of
ground, the whole step reads as grade. The floor a window must clear to hold
a target grade precision is:

~~~text
W >= altitude_resolution / target_grade_precision
~~~

Worked example from the handover document
([power-estimation-handover.md](../references/power-estimation-handover.md)
§3): 0.2 m of altimeter resolution over a target precision of 0.2 percentage
points gives 0.2 / 0.002 = 100 m.

**Constants.** W = 100 m for a route's steepest gradient
(`internal/route/route.go` `gradientWindowMetres`) and for the browser's
bands and climbs (`internal/webui/app/src/lib/profile.ts`
`GRADIENT_WINDOW_METRES`); for the estimated-power model W is derived per
ride as `clamp(quantum / 0.002, 30 m, 300 m)`, where quantum is the smallest
positive altitude step between consecutive samples whose clock advanced by
no more than the recording gap, rounded to a hundredth of a metre so a
floating-point 0.19999 reads as 0.2 (`internal/measure/estimate.go` `gradeWindowMetres`,
`targetGradePrecision`, `minWindowMetres`, `maxWindowMetres`) — 100 m for a
0.2 m barometric altimeter.

**Source.** The 100 m floor is derived above from the handover document's
own reasoning. This service's own rule for choosing to apply that floor to
`gradientWindowMetres` and `GRADIENT_WINDOW_METRES`, and for deriving the
estimated-power model's window from each ride's own altimeter instead.

**Applied by.** `route.Route.MaxGradientPercent` through
`measure.Profile.MaxGradientPercent` at `gradientWindowMetres`
(`internal/measure/profile.go`).

**Status.** 100 m meets the derived floor and is validated for routes and
the browser. The estimated-power model's known deviation is resolved: its
window is no longer fixed below the floor but derived from each ride's own
altimeter resolution, landing at 100 m for a typical 0.2 m barometer.

## Ascent and descent

**Definition.** The sum of positive adjacent altitude steps (ascent), or the
sum of negative ones (descent), unsmoothed.

**Formula.** In symbols:

~~~text
ascent  = Σ max(elevation[i] - elevation[i-1], 0)
descent = Σ max(elevation[i-1] - elevation[i], 0)

with hysteresis T: a climb opens once the series has risen T above its
lowest point since the last climb closed, and closes once it has fallen T
below its peak; ascent = Σ (peak - trough) over the climbs so closed, the
last one closed by the end of the series. Descent is the same walk over
the falls.
~~~

**Constants.** None beyond the profile the sum runs over for a route. For a
ride's own barometric samples the hysteresis threshold is 3 m
(`internal/activity/splits.go` `splitAscentHysteresisMetres`), the value
that agrees with the head unit's own figure to within a few per cent over
the operator's rides (#608); a route's provider profile needs none, and
gets none.

**Source.** This service's own rule for the sums; the hysteresis walk is
the accumulation-threshold altimeter of US patent 5,058,427 and what every
head unit approximates. That the figure depends on the scale it is measured
at, and never converges, is Rapaport 2011; that a barometric device
over-reports and a GPS device under-reports a surveyed climb, and that a
device's figure is not the ground, is Sánchez and Villena 2020 and Menaspà
et al. 2016.

**Applied by.** `route.Route.ElevationGainMetres` and `ElevationLossMetres`
through the package-level `measure.AscentMetres` and `DescentMetres`
(`internal/measure/profile.go`, which `Profile`'s methods of the same name
also call), run on the median-filtered profile (see
Profiles above), which is the only profile this sum is meaningful on: raw
satellite altitude noise summed over thousands of points inflates the total
badly, per `ElevationGainMetres`'s own comment. `internal/activity/splits.go` counts a
ride split's ascent with `measure.AscentWithHysteresisMetres` at 3 m over
each unbroken run of the stretch's own samples that carried a height, the
first opened by the sample the stretch began from where it carried one; a
sample without a height breaks the run rather than bridging it, and a split
reports no descent.

**Calibration coupling.** `ridemodel.Predict` prices a route's raw-step
ascent on the median-filtered profile (`internal/ridemodel/model.go`), while
the weekly calibration that fits `seconds_per_ascent_m`
(`internal/ridemodel/calibrate.go`, fed by `activities.ascent_metres`) fits
against the ascent Wahoo's device reported for each ride, which the device
counted with its own threshold. The prediction and its calibration therefore
price different definitions of ascent today, and the ascent study
(`dev/ascentstudy`, #608) measures how far apart they are on the rider's own
rides. The offline fitter that once benchmarked the model against a Strava
export was retired with the model's calibration moving into the service.

**Other platforms.** Strava and Intervals.icu both count ascent with a
hysteresis threshold: Strava uses 2 m with barometric data and 10 m without
it (Strava "Elevation" and "Elevation on Strava FAQs"); Intervals.icu uses
1.2 m (Intervals.icu forum "Elevation gain off?"). This service's walk is
in `measure.AscentWithHysteresisMetres` and `DescentWithHysteresisMetres`
(`internal/measure/profile.go`), called by nobody yet.

**Status.** Settled by the ascent study (#608, `dev/ascentstudy`) over the
operator's rides. A ride's own barometric samples need the 3 m walk: the
raw sum over-reports the head unit by about three quarters, the walk lands
within a few per cent. A route's stored profile needs no walk: it already
reads about 5 % under the head unit for the same ground, and every
threshold pushes it further under. Neither figure is the ground: a head
unit reads 2–5 % under a surveyed climb in dry weather and far more in rain
(Menaspà et al. 2016), so a route summary sits roughly 8–10 % under the
truth and a ride's figure a few per cent under it; on the operator's rides
rain made no visible difference to the head unit's figure. The prediction
prices the route figure and the weekly fit measures against the head
unit's, a consistent 5 % apart that the coefficient absorbs; nothing is
refitted on that account.

## Recording gaps

**Definition.** A step to the next sample longer than a fixed gap marks a
pause and ends a stretch of recording. A step that is not positive is
skipped where it stands, holding nothing, but does not end the stretch. A
sample stands for the time until the next one; the last sample stands for
nothing.

**Formula.** In symbols:

~~~text
held(i) = at[i+1] - at[i],  counted only where 0 < held(i) <= maxGap
mean    = Σ value[i]·held(i)  /  Σ held(i),  over counted i only
~~~

**Constants.** maxGap = 10 s (`measure.DefaultMaxGap`,
`internal/measure/gap.go`).

**Source.** This service's own rule.

**Applied by.** `trainingload.TRIMP`, `HeartRateTSS` and `TimeInZones`
through `measure.ForEachHeld`/`MeanHeld`; `measure.Stretches` and
`measure.EstimateSeries` are the same rule in `internal/measure/gap.go`.

**Status.** Validated: this is the gap rule live training-load figures use
today.

## Rolling mean

**Definition.** For each sample with a full window of recording behind it,
the mean over the shortest span that still covers the window length asked
for, never reaching across a pause.

**Formula.** In symbols:

~~~text
for each end sample e with a full window behind it:
  start = the latest sample s such that at[e] - at[s] >= windowLength
  mean(e) = ∫ value dt over [s, e]  /  (at[e] - at[s])
~~~

Normalised power raises this mean to the fourth power before taking its own
mean over the ride; a best-average power takes the maximum over the ride
instead of a mean.

**Constants.** windowLength = 30 s for normalised power
(`internal/trainingload/load.go` `normalizedPowerWindow`); windowLength =
20 min where `internal/rider/best.go` `BestAverage` is called for a
best-average power (the window is a caller-supplied parameter, not a
constant in that package).

**Source.** Coggan (see Training load below) for the 30 s normalised-power
window; 20 minutes is this service's own choice of a conventional
best-average duration.

**Applied by.** `trainingload.PowerLoad` and `rider.BestAverage`, both through
`measure.RollingMean` (`internal/measure/gap.go`); the private copies each
package once carried are gone.

**Status.** Validated: these are the figures live training-load and
best-average pages show today.

## Estimated power

**Definition.** A force-balance model of the power needed to hold a
recorded speed over a recorded grade, for rides with no power meter.

**Formula.** In symbols:

~~~text
P = 0                                              where cadence is known and zero
P = v · (m·g·grade + m·g·Crr + ½·ρ·CdA·v²),  clamped at 0,   otherwise

p = 101325 · (1 - 2.25577e-5 · h)^5.25588          (h in metres)
ρ = p / (287.058 · (T + 273.15))                   (T in °C)
~~~

v is speed in m/s, grade is the dimensionless rise over run from the
Gradient section above (measured over the window §Gradient derives), m is
total system mass in kg. The cadence rule is checked first and is physics,
not the zero clamp below it: a sample the rider was not pedalling through
has no power to estimate, whatever the track says about grade and speed at
that moment. It contributes nothing to the clamp's own bias diagnostic
below, since the clamp never had a chance to fire on it. ρ is evaluated at
the window's high sample's altitude and its temperature where the sample
carries one.

**Constants.** g = 9.80665 m/s², Crr = 0.005, CdA = 0.32 m²
(`internal/measure/estimate.go` `gravity`, `rollingResistance`, `dragArea`).
A sample with no temperature reading is evaluated at 15 °C, the value the
model's density used to be fixed at
(`internal/measure/estimate.go` `defaultTemperatureCelsius`).

**Source.** Martin et al. 1998. The source model also carries three terms
this service omits, each at the source's own value: drivetrain efficiency
η = 0.977 (the source divides the whole force sum by η rather than omitting
it); rotational inertia as an added 1.5 kg of equivalent linear mass in the
inertial term; and wind as a signed `|v+w|·(v+w)` aerodynamic term rather
than squaring `v` alone, so a tailwind faster than the rider still drags
correctly instead of reading as a spurious push
(`internal/measure/estimate.go`'s own comment already states drivetrain
loss is left out as "a couple of per cent on a figure already labelled an
estimate"). Air density follows the handover document's own formula
(§2) rather than the fixed sea-level, fifteen-degree constant this service
used to use; the cadence gate is the handover's own §5 rule, kept distinct
from the zero clamp for the same reason the handover gives: conflating them
hides how much of the estimate the clamp is inventing.

**Quality diagnostics.** The handover document
([power-estimation-handover.md](../references/power-estimation-handover.md)
§7) gates its output on three self-diagnosing checks: lag-1 autocorrelation
greater than +0.8, mean absolute second-to-second power change under 40 W,
and a clipping bias under roughly 8 W. A series failing them should report
average power and energy only, because normalised power and best-average
figures both take a maximum or a fourth power and so amplify noise rather
than average it away. Computed by `EstimateSeries` and returned beside the
series; stored on the ride's metrics row alongside the estimate
(`activity_metrics.estimate_autocorrelation`,
`activity_metrics.estimate_delta_watts_per_second`,
`activity_metrics.estimate_clip_bias_watts`), served as `estimateQuality`
beside `estimatedPowerWatts`, and shown on the ride page. A ride derived
before those columns existed holds nulls in them and is served without an
`estimateQuality` until the bumped derivation version lists it again.
The three thresholds above remain the handover's own targets, not a rule this
service enforces: nothing in this service gates on the diagnostics yet.

**Applied by.** `measure.EstimateSeries`, called by
`activity.RideSamples.EstimatePower`.

**Status.** Unvalidated against a power meter. The handover document's
one-ride validation, on a different bicycle and rider than this service's
own, is the only evidence behind the constants above.

## Sustained climbs

**Definition.** A run of the route where the signed gradient, measured back
over the same window as Gradient above, stays at or above the flat/not-flat
edge, reported once it is at least a window long.

**Formula.** In symbols:

~~~text
gradient(i) = signed grade over the 100 m look-back window, as in Gradient
climbing(i) = gradient(i) >= 3%
runs        = coordinates split into consecutive climbing / not-climbing runs
runs shorter than 100 m are absorbed into the run before them
a climb is reported for each remaining climbing run at least 100 m long

ascent      = Σ positive steps inside the climb
averageGrade = (endElevation - startElevation) / climbLength · 100
maxGrade    = the steepest 100 m window found inside the climb
~~~

**Constants.** 3% is `GRADIENT_BANDS[0].limit`, the first gradient band's
limit (`internal/webui/app/src/lib/profile.ts`); the 100 m window is the
same `GRADIENT_WINDOW_METRES` as Gradient above
(`internal/webui/app/src/lib/climbs.ts` `CLIMB_GRADIENT_PERCENT`,
`MIN_CLIMB_METRES`).

**Source.** This service's own rule.

**Applied by.** `findClimbs` in the browser
(`internal/webui/app/src/lib/climbs.ts`) and `measure.Climbs` in Go, the
same rule.

**Status.** Both implementations exist and are pinned to each other by a
shared table of vectors in `climbs.test.ts` and `climb_test.go`; nothing
serves the Go result yet.

## Sensor cleaning

**Definition.** Heart-rate samples above the rider's maximum, replaced by
linear interpolation between the readings either side. Power samples
clamped at an implausibility threshold.

**Formula.**

~~~text
hr'(t) = hr(a) + (hr(b) - hr(a)) · (t - a)/(b - a)
~~~

for a spike run between plausible readings at a and b, taking the nearest
plausible value where a run reaches either end of the series; and

~~~text
p' = min(p, maxWatts)
~~~

**Constants.** None fixed in `measure`; maxBPM is the rider profile's
maximum heart rate and maxWatts is a caller's threshold, neither chosen yet.

**Source.** Intervals.icu forum, "Heartrate spikes now automatically fixed"
(January 2020 announcement), for the heart-rate interpolation approach.

**Applied by.** `measure.CapHeartRate` and `measure.ClampPower`.
`activity:derive` (`internal/activity/derive.go` `deriveMetrics`) calls
`measure.CapHeartRate` with the rider profile's maximum heart rate before any
load is derived from the series; `ClampPower` is still called by nobody —
no implausibility threshold has been chosen. The one rule this service
otherwise applies sits in `internal/activity/averages.go` `meanAndPeak`: a
cadence reading of zero is left out of a ride's average cadence, while a
heart-rate or power reading of zero is kept in its average, because a bicycle
freewheeling or a rider resting is still riding, but a cadence sensor reading
zero recorded no pedalling to average in.

**Status.** `CapHeartRate` is implemented, tested and wired into
`activity:derive`. `ClampPower` remains implemented and tested but unwired;
wiring it is a behaviour change that lands with a revision of service.md
§Recorded activities once a threshold is chosen.

## Training load

**Definition.** Everything `internal/trainingload` derives from one ride's
recorded samples and one rider's profile.

**TRIMP.** Banister's training impulse:

~~~text
TRIMP = Σ minutes(held) · f · 0.64 · e^(1.92·f)
f     = clamp((heartRate - restingHeartRate) / (maxHeartRate - restingHeartRate), 0, 1)
~~~

summed only over held seconds (see Recording gaps above), with 0.64 and
1.92 the men's exponential-weighting coefficients
(`internal/trainingload/load.go` `trimpFactor`, `trimpExponent`). The
rider's sex is not among this service's profile fields, so the men's
coefficients are used for every rider; the code's own comment states the
reason: they scale every ride by the same constant, and a rider reads TRIMP
against their own other rides rather than against anybody else's.

**Heart-rate TSS.**

~~~text
IF  = clamp((meanHeartRate - restingHeartRate) / (thresholdHeartRate - restingHeartRate), 0, ∞)
TSS = hours(held) · IF² · 100
~~~

using the time-weighted mean over held seconds from Recording gaps, and the
share of threshold reserve rather than the bare ratio of mean to threshold
rate, because heart rate does not fall to zero the way power does
(`internal/trainingload/load.go` `HeartRateTSS`).

**Normalised power.**

~~~text
NP  = fourth-root of the mean, over held seconds, of (rolling 30 s mean power)^4
IF  = NP / FTP
TSS = heldSeconds · NP · IF / (FTP · 3600) · 100
~~~

using the Rolling mean section's 30 s window
(`internal/trainingload/load.go` `PowerLoad`, via `measure.RollingMean`).

**Fitness, fatigue, form.** Two exponential moving averages of daily load,
42 days and 7 days (`internal/trainingload/fitness.go` `FitnessDays`,
`FatigueDays`), each day moving the average towards that day's own load by
one part in the number of days:

~~~text
average(day) = average(day-1) + (load(day) - average(day-1)) / days
form(day)    = fitness(day) - fatigue(day)
~~~

A day with no ride still carries a load of zero into this update, which is
what lets rest turn accumulated load into form.

**Heart-rate zones.** Five zones cut by four bounds, from a threshold rate
where the rider has entered one (0.85, 0.90, 0.95, 1.00 of threshold) or
from a maximum rate otherwise (0.60, 0.70, 0.80, 0.90 of maximum)
(`internal/trainingload/zones.go` `thresholdShares`, `maximumShares`,
`BoundsFrom`). The threshold scheme is preferred because a threshold is
measured and a maximum is often guessed.

**Source.** TRIMP: Banister 1991. Heart-rate TSS and power TSS/IF/NP:
Coggan, in Allen and Coggan 2010.

**Applied by.** `internal/trainingload/load.go` (`TRIMP`, `HeartRateTSS`,
`PowerLoad`), `internal/trainingload/zones.go` (`BoundsFrom`, `TimeInZones`),
`internal/trainingload/fitness.go` (`Timeline`, `decay`).

**Status.** Validated: these are the figures a rider's own training-load
pages show today.

## Decoupling and heat drift

Two readings taken over a ride's own sensors, both from **measured power
only**. An estimated power carries a per-ride bias (see §Estimated power), and
either figure fed by one would compare a ride against a differently biased
version of itself.

Both read the **cleaned** heart-rate series (§Sensor cleaning), not the ride's
raw one. A spike above the rider's maximum falls in one half, and read raw it
would be drift that never happened.

**Aerobic decoupling.** The ride is split in half by elapsed time. Each half's
power-to-heart-rate ratio is its mean measured power over its mean heart rate,
and decoupling is how much of the first half's ratio the second half lost, as a
percentage:

~~~text
decoupling = (ratio_first - ratio_second) / ratio_first * 100
~~~

Positive is the usual direction: the same watts cost more beats later on. A
tenth more beats for the same power is a decoupling of 9.09%, not of 10%.

**Constants.** The ride's recorded samples must span at least **one hour**,
first to last, for the two halves to be worth comparing. The span is what is
measured rather than the ride's own elapsed time, so a ride recorded once a
second runs a second past the hour before it clears the bar. Absent below that,
and absent for a ride carrying no measured power or no heart rate.

**What it does not say.** The figure describes a *steady* aerobic ride. Over
intervals the two halves are different efforts and the number is not drift.
This service does not test a ride for steadiness, so the reader is told the
scale rather than sold the interpretation.

**Heat drift.** Per ride, the mean heart rate over the samples whose measured
power falls inside the rider's endurance band, paired with the mean temperature
over those same samples. The pair is a point; the drift is the points over a
season, which is why both halves are stored and neither is a trend on its own.

**Constants.** The band is **55% to 75% of the rider's threshold power**, which
is where heart rate answers temperature rather than the effort. At least **300
samples** must fall inside it, so a ride that merely passed through the band is
not a reading of it. Absent without a threshold power to place the band, without
measured power, without a thermometer, or below that sample count.

Heart rate and temperature are paired by the second each was recorded at, which
is the resolution the records are stored at and so the only basis on which two
sensors are known to describe the same moment.

**Source.** Decoupling is Friel's Pw:Hr [17]. Heat drift has no single source:
it is the plain pairing this service stores, and the interpretation is left to
the reader.

**Applied by.** `internal/activity/drift.go` (`Decoupling`, `HeatDrift`).

**Status.** Unvalidated against an independent implementation. The formulas are
covered by unit tests over synthetic streams with a known drift; no ride's
figure has been checked against another platform's.

## Power-duration curve

The best mean power a rider held at each of a fixed set of durations, over a
window of their own rides. **Measured power only** — an estimate from the track
is a different kind of number, and a curve is the figure a rider compares
against other riders' meters.

**Constants.** Six durations, shortest first: **5 s, 30 s, 1 min, 5 min, 20 min
and 1 hour**. The 20-minute point is the same window the threshold-power
suggestion is taken over, deliberately: the two are 95% of the same best twenty
minutes and must not drift apart.

**How it is built.** Each ride's best over each duration is worked out once, in
the derivation pass, by the same rolling window §Rolling mean defines and stored
beside the ride's other derived figures. The curve served for a window is the
maximum of those stored bests across the rides in it, so a read folds stored
numbers rather than rescanning every sensor sample the rider has recorded.

A ride shorter than a duration holds no best for it, which is what leaves the
long end of a curve empty until a long ride arrives. A duration no ride reached
carries no point rather than a nought.

**The threshold suggestion is not read off the curve**, though both are 95% of
the same best twenty minutes. A derivation runs only for a rider who has entered
something, so the curve is empty for a rider with no profile at all — who is
exactly the rider a threshold is suggested to. The two can differ only while a
ride's samples are stored and its derivation is still owed.

**Applied by.** `internal/activity/powercurve.go` (`PowerBests`),
`internal/sqlite/metrics.go` (`PowerCurve`).

**Status.** Unvalidated against another platform's curve. The per-ride bests are
covered by unit tests over synthetic streams with known bests.

## References

1. Sinnott, R. W. (1984) "Virtues of the Haversine", Sky and Telescope
   68(2):158.
2. Martin, J. C., Milliken, D. L., Cobb, J. E., McFadden, K. L., Coggan,
   A. R. (1998) "Validation of a Mathematical Model for Road Cycling
   Power", Journal of Applied Biomechanics 14(3):276-291.
3. Danek, T. et al. (2020) arXiv:2005.04229 and arXiv:2005.04480
   (least-squares estimation of CdA, Crr and drivetrain loss).
4. Banister, E. W. (1991) "Modeling elite athletic performance" in
   Physiological Testing of Elite Athletes.
5. Allen, H. and Coggan, A. (2010) Training and Racing with a Power Meter,
   2nd ed.
6. The internal handover document at
   [docs/references/power-estimation-handover.md](../references/power-estimation-handover.md).
7. Strava "Elevation"
   <https://support.strava.com/en-us/articles/15401909-elevation>, accessed
   2026-09-07.
8. Strava "Elevation on Strava FAQs"
   <https://support.strava.com/hc/en-us/articles/115001294564-Elevation-on-Strava-FAQs>,
   accessed 2026-09-07.
9. Strava "How to get power for your rides"
   <https://support.strava.com/en-us/articles/15401944-how-to-get-power-for-your-rides>,
   accessed 2026-09-07.
10. Intervals.icu forum "Elevation gain off?"
    <https://forum.intervals.icu/t/elevation-gain-off/8463>, accessed
    2026-09-07.
11. Intervals.icu forum "Heartrate spikes now automatically fixed"
    <https://forum.intervals.icu/t/heartrate-spikes-now-automatically-fixed/174>,
    accessed 2026-09-07.
12. Intervals.icu forum "Estimating load for rides without power meters"
    <https://forum.intervals.icu/t/estimating-load-for-rides-without-power-meters/20863>,
    accessed 2026-09-07.
13. Rapaport, D. C. (2011) "Evaluating cumulative ascent: Mountain biking
    meets Mandelbrot", arXiv:1011.4778 [physics.data-an],
    <https://arxiv.org/abs/1011.4778>.
14. Menaspà, P., Haakonssen, E., Sharma, A., Clark, B. (2016) "Accuracy in
    measurement of elevation gain in road cycling", Journal of Science and
    Cycling 5(1):10–12, CC BY 3.0; reproduced as
    [menaspa-2016-elevation-gain-road-cycling.pdf](../references/menaspa-2016-elevation-gain-road-cycling.pdf).
15. Sánchez, R., Villena, M. (2020) "Comparative evaluation of wearable
    devices for measuring elevation gain in mountain physical activities",
    Proceedings of the Institution of Mechanical Engineers, Part P: Journal
    of Sports Engineering and Technology, doi:10.1177/1754337120918975.
16. US patent 5,058,427 (1991) "Accumulating altimeter with ascent/descent
    accumulation thresholds", <https://patents.justia.com/patent/5058427>.
17. Friel, J. "Aerobic Endurance Testing", josephfriel.com,
    <https://josephfriel.com/aerobic-endurance-testing/>; the Pw:Hr
    decoupling ratio as used by TrainingPeaks.
