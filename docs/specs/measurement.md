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
`route` itself (route length, `MaxGradientPercent`), `elevation`
(resampling), `fit`, `wahoo`, `surface`, `ridemodel`, `demo`.
`route.CumulativeMetres` is the running sum over a geometry, built on
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

**Applied by.** `internal/elevation/normalizer.go` `resampleElevations`,
`applyMovingMedian`, before device export; `measure.Profile.Resample`,
`MedianFiltered` and `AltitudeAt` are the same steps in
`internal/measure/profile.go`, which `elevation` moves onto.

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
`GRADIENT_WINDOW_METRES`); W = 30 m for the estimated-power model
(`internal/powerestimate/estimate.go` `windowMetres`).

**Source.** The 100 m floor is derived above from the handover document's
own reasoning. This service's own rule for choosing to apply that floor to
`gradientWindowMetres` and `GRADIENT_WINDOW_METRES`.

**Applied by.** `route.Route.MaxGradientPercent` today;
`measure.Profile.GradientsPercent` and `MaxGradientPercent` are the same
walk in `internal/measure/profile.go`, which `route` moves onto.

**Status.** 100 m meets the derived floor and is validated. 30 m is a known
deviation: it sits below the floor, so 0.2 m of altimeter noise over 30 m of
travel is 0.2/30 = 0.67% of grade, which the power-estimate model turns into
roughly 40 W at 95 kg and 7 m/s. This is to be revisited.

## Ascent and descent

**Definition.** The sum of positive adjacent altitude steps (ascent), or the
sum of negative ones (descent), unsmoothed.

**Formula.** In symbols:

~~~text
ascent  = Σ max(elevation[i] - elevation[i-1], 0)
descent = Σ max(elevation[i-1] - elevation[i], 0)
~~~

**Constants.** None beyond the profile the sum runs over.

**Source.** This service's own rule.

**Applied by.** `route.Route.ElevationGainMetres` and `ElevationLossMetres`
run on the median-filtered profile (see Profiles above), which is the only
profile this sum is meaningful on: raw satellite altitude noise summed over
thousands of points inflates the total badly, per
`ElevationGainMetres`'s own comment. `internal/activity/splits.go`
`splitParts.add` runs the same positive-step sum on raw recorded samples for
a ride split's ascent — the code carries no matching descent sum for splits,
which disagrees with what this section otherwise describes as symmetric; a
split's `AscentMetres` is the only figure `Split` reports.

**Calibration coupling.** `internal/ridemodel`'s `seconds_per_ascent_m`
coefficient was fitted against `route.Route.ElevationGainMetres`'s
definition (`internal/ridemodel/model.go`); changing that definition forces
a refit.

**Other platforms' step two.** Strava and Intervals.icu both apply a
hysteresis threshold after summing steps, which this service does not:
Strava uses 2 m with barometric data and 10 m without it (Strava
"Elevation" and "Elevation on Strava FAQs"); Intervals.icu uses 1.2 m
(Intervals.icu forum "Elevation gain off?"). These are candidates for a
second step, not something this service does today.

**Status.** Known deviation from hysteresis practice.
`measure.Profile.AscentMetres` and `DescentMetres` are the same sums in
`internal/measure/profile.go`, which `route` moves onto.

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

**Constants.** maxGap = 10 s
(`internal/trainingload/zones.go` `maxSampleGap`; the same 10 s appears
separately in `internal/powerestimate/estimate.go` `maxSampleGap` and
`internal/rider/best.go` `maxSampleGap`).

**Source.** This service's own rule.

**Applied by.** `internal/trainingload/zones.go` `forEachHeld`, `meanHeld`,
and `TimeInZones`'s use of them; `measure.DefaultMaxGap`, `Stretches`,
`ForEachHeld` and `MeanHeld` are the same rule in `internal/measure/gap.go`,
which `trainingload`, `powerestimate` and `rider` move onto.

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

**Applied by.** `internal/trainingload/load.go` `rollingFourthPowerMean` and
`internal/rider/best.go` `BestAverage`; `measure.RollingMean` is the one
window both become in `internal/measure/gap.go`.

**Status.** Validated: these are the figures live training-load and
best-average pages show today.

## Estimated power

**Definition.** A force-balance model of the power needed to hold a
recorded speed over a recorded grade, for rides with no power meter.

**Formula.** In symbols:

~~~text
P = v · (m·g·grade + m·g·Crr + ½·ρ·CdA·v²),  clamped at 0
~~~

v is speed in m/s, grade is the dimensionless rise over run from the
Gradient section above (measured over the 30 m window), m is total system
mass in kg.

**Constants.** g = 9.80665 m/s², Crr = 0.005, CdA = 0.32 m², ρ = 1.225 kg/m³
(`internal/powerestimate/estimate.go` `gravity`, `rollingResistance`,
`dragArea`, `airDensity`).

**Source.** Martin et al. 1998. The source model also carries three terms
this service omits, each at the source's own value: drivetrain efficiency
η = 0.977 (the source divides the whole force sum by η rather than omitting
it); rotational inertia as an added 1.5 kg of equivalent linear mass in the
inertial term; and wind as a signed `|v+w|·(v+w)` aerodynamic term rather
than squaring `v` alone, so a tailwind faster than the rider still drags
correctly instead of reading as a spurious push
(`internal/powerestimate/estimate.go`'s own comment already states drivetrain
loss is left out as "a couple of per cent on a figure already labelled an
estimate"). The handover document treats air density as a function of
altitude and temperature rather than the fixed sea-level, fifteen-degree
constant this service uses.

**Quality diagnostics (not yet computed).** The handover document
([power-estimation-handover.md](../references/power-estimation-handover.md)
§7) gates its output on three self-diagnosing checks: lag-1 autocorrelation
greater than +0.8, mean absolute second-to-second power change under 40 W,
and a clipping bias under roughly 8 W. A series failing them should report
average power and energy only, because normalised power and best-average
figures both take a maximum or a fourth power and so amplify noise rather
than average it away. This service does not compute these diagnostics
today.

**Applied by.** `internal/powerestimate/estimate.go` `Series` today, to
become `measure.EstimateSeries` when the package moves.

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

**Applied by.** `findClimbs` in `internal/webui/app/src/lib/climbs.ts`
today. A Go port, `measure.Climbs`, is planned; the browser and Go
implementations must agree the way the two `HaversineMetres` copies do.

**Status.** Validated as the browser's live behaviour; unvalidated as a
cross-implementation agreement, since the Go port does not exist yet.

## Sensor cleaning

**Definition (planned, not implemented).** Heart-rate samples above the
rider's maximum, replaced by linear interpolation between the readings
either side. Power samples clamped at an implausibility threshold.

**Formula.** Not yet chosen.

**Constants.** Not yet chosen.

**Source.** Intervals.icu forum, "Heartrate spikes now automatically fixed"
(January 2020 announcement), for the heart-rate interpolation approach.

**Applied by.** Nobody yet. The one rule this service does apply today sits
in `internal/activity/averages.go` `meanAndPeak`: a cadence reading of zero
is left out of a ride's average cadence, while a heart-rate or power reading
of zero is kept in its average, because a bicycle freewheeling or a rider
resting is still riding, but a cadence sensor reading zero recorded no
pedalling to average in.

**Status.** Unvalidated; not implemented.

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
(`internal/trainingload/load.go` `PowerLoad`, `rollingFourthPowerMean`).

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
`PowerLoad`, `rollingFourthPowerMean`), `internal/trainingload/zones.go`
(`BoundsFrom`, `TimeInZones`), `internal/trainingload/fitness.go`
(`Timeline`, `decay`).

**Status.** Validated: these are the figures a rider's own training-load
pages show today.

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
