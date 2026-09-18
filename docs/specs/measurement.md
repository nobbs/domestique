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
the browser. The estimated-power model's window is derived from each ride's
own altimeter resolution, landing at 100 m for a typical 0.2 m barometer.

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
price different definitions of ascent today, measured on the rider's own
rides.

**Other platforms.** Strava and Intervals.icu both count ascent with a
hysteresis threshold: Strava uses 2 m with barometric data and 10 m without
it (Strava "Elevation" and "Elevation on Strava FAQs"); Intervals.icu uses
1.2 m (Intervals.icu forum "Elevation gain off?"). This service's walk is
in `measure.AscentWithHysteresisMetres` and `DescentWithHysteresisMetres`
(`internal/measure/profile.go`), called by nobody yet.

**Status.** Settled over the operator's own rides (#608). A ride's own barometric samples need the 3 m walk: the
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

**Applied by.** `trainingload.TRIMP`, `HeartRateTSS`, `TimeInZones` and
`TimeAtHeartRate` through `measure.ForEachHeld`/`MeanHeld`; `measure.Stretches`
and `measure.EstimateSeries` are the same rule in `internal/measure/gap.go`.

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

**Definition.** The power a rider was producing at each recorded sample, worked out from the ride's own track by a force balance, for a bicycle carrying no meter; and the ride's figure, the mean of it over the samples the rider was pedalling through, beside the share of the ride that was.

**Formula.** Samples are cut into stretches wherever the clock advances by more than the recording gap (10 s) or the recorded distance runs backward (an odometer reset); nothing is measured across either boundary. Around sample $i$, the window $[lo, hi]$ starts at $i$ and each round moves $lo$ down and $hi$ up by one sample together, stopping as soon as the covered distance reaches $W$ metres or an edge meets its own stretch's boundary (W from §Gradient, 100 m for a 0.2 m barometer) — centred on $i$ rather than the smallest span that would cover $W$, so unevenly spaced samples never bias the reading toward one side. Then

$$v_i = \frac{d_{hi} - d_{lo}}{t_{hi} - t_{lo}}, \qquad \text{grade}_i = \frac{h_{hi} - h_{lo}}{d_{hi} - d_{lo}}, \qquad a_i = \frac{v_i - v_j}{t_i - t_j}$$

where $j$ is the first sample at least 10 s behind $i$ in the same stretch, and $a_i = 0$ where the stretch reaches back no further. Air density at the window's high sample:

$$p = 101325\,(1 - 2.25577\times10^{-5}\, h)^{5.25588}, \qquad \rho = \frac{p}{287.058\,(T + 273.15)}$$

with $h$ in metres and $T$ in °C, 15 °C where the sample carries no thermometer. The power:

$$P_i = \frac{v_i}{\eta}\left(m g\,\text{grade}_i + m g\, C_{rr} + \tfrac{1}{2}\rho\, C_dA\, v_i\,|v_i| + (m + m_{rot})\, a_i\right)$$

$P_i = 0$ where the cadence is known and zero (a rider who is not pedalling produces nothing, whatever the track says); otherwise $P_i$ is clamped at zero (a rider cannot absorb power). $m$ is the total system mass, rider plus bicycle. The ride's figure, over the pedalling set $S$ = samples with a known estimate whose cadence is unknown or above zero:

$$\bar P = \frac{1}{|S|}\sum_{i \in S} P_i, \qquad \text{share} = \frac{|S|}{\text{samples with a known estimate}}$$

**Constants.** $g$ = 9.80665 m/s², $\eta$ = 0.977 (drivetrain), $m_{rot}$ = 1.5 kg (equivalent linear mass of the wheels' rotational inertia, in the inertial term only), 10 s acceleration baseline, 15 °C default temperature, the recording gap and window from their own sections. $C_dA$ and $C_{rr}$ are the rider's own bicycle, entered on the rider profile (`dragAreaM2`, `rollingResistance`); a profile without them is estimated at a road bicycle on the hoods, $C_dA$ = 0.36 m², $C_{rr}$ = 0.005. Guidance rows from the handover table: hoods 0.36, gravel bike on the hoods 0.40, sitting up 0.45 m²; slick road tyre 0.005, wide gravel tyre on tarmac 0.008. Code: `internal/measure/estimate.go` (`gravity`, `drivetrainEfficiency`, `rotationalMassKG`, `accelerationBaseline`, `defaultTemperatureCelsius`, `DefaultCoefficients`).

**Source.** Martin et al. 1998 [2] for the force balance, its drivetrain efficiency and rotational inertia; the handover document [5] for the coefficient table and the window derivation.

**Applied by.** `measure.EstimateSeries` for the series and `measure.PedallingMean` for the ride figure and share, called by `activity.RideSamples.EstimatePower` in `activity:derive`; stored per record as `activity_records.estimated_power_watts` and per ride as `activity_metrics.estimated_power_watts` and `estimated_pedalling_share`; served as `estimatedPowerWatts` and `estimatedPedallingShare`. The estimate is named as one everywhere, is never mixed with measured power and never feeds a training load, a power curve or a normalized power. A climb attempt's estimated mean is the mean of the stored series over the climb.

**Status.** Validated against the operator's own trainer power at matched heart rate: ride means within 6% on rides held out from the check, which `dev/levelstudy` reproduces from a state snapshot. The same tool sets the estimate beside the meter month by month over the months holding both indoor and outdoor rides; as of September 2026, over seven such months, no statistic is steady enough to anchor a scale or a coefficient correction on, so the estimate stays unanchored (#723). A normalized power of the estimate is not served: on trainer tracks, at a drag area fitted to the meter, it reads a median 5 % over the meter's own (quartiles 0 to 11 %), and its excess over its mean is nearly twice the meter's, which `dev/npstudy` reproduces (#702).

## Estimated calories

**Definition.** The energy a ride's average power implies at a fixed gross efficiency, served beside the calorie figure the device itself reported, purely so a rider can compare the two.

**Formula.** In symbols:

~~~text
kJ   = averageWatts · seconds / 1000
kcal = kJ / 4.184 / 0.22
~~~

Where the ride measured its own power, `averageWatts` is that measured average and `seconds` is the ride's whole moving time. Otherwise `averageWatts` is the ride's own Estimated power above, which is a mean over the pedalling samples only — so `seconds` is the moving time scaled by that estimate's own pedalling share, landing the energy on the same time base the mean was taken over. Absent below a positive wattage and moving time, and for an estimate carrying no positive pedalling share of its own.

**Constants.** 4.184 (kJ per kcal, physical) and 0.22 (assumed gross cycling efficiency), both from the handover document's own cross-check. Code: `internal/measure/energy.go` (`kilojoulesPerKilocalorie`, `grossCyclingEfficiency`).

**Source.** The handover document [5] §10, "Cross-checks to implement".

**Applied by.** `measure.EstimatedCalories`, called from `internal/httpapi/routes_activities.go` when an activity is served; never stored, never an input to a training load. Served as `estimatedCaloriesKcal`, which never replaces the activity's own `caloriesKcal` — beside it where the device also reported one, and on its own where it did not.

**Status.** Unvalidated, and not fully validatable: the handover document's own worked example notes the device's reported calories is itself usually heart-rate-derived rather than an independent measurement of the same thing this formula estimates. A comparison aid, not a checked figure.

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

**Constants.** 3% is the first gradient band's limit, as Gradient above
defines it, and the 100 m window is that section's `GRADIENT_WINDOW_METRES`.
Both are named in `internal/activity/climbattempt.go`
(`ClimbMinGradientPercent`, `ClimbWindowMetres`), which is where the rule is
applied from.

**Source.** This service's own rule.

**Applied by.** `measure.Climbs`, once. The browser drew the same rule for
itself until the service began timing rides over these climbs; two
implementations of one rule can only come to disagree, and a climb the two put
in different places is a rider's time shown against the wrong hill. The browser
now draws the climbs the route serves.

**Status.** Validated against the vectors in `climb_test.go`.

## Ahead of prediction

**Definition.** How far ahead of its route's predicted moving time a ride was
at a place along the route: the predicted time from where the ride joined the
route to that place, less the ride's own moving time over the same. Positive
is ahead, zero is on the prediction.

**Formula.** In symbols:

~~~text
passes(i)    = the route's segments within the 40 m route-matching corridor of
               sample i, grouped into places more than 80 m apart along it,
               each at its nearest segment, in the snap index's projected frame
along(i)     = the pass nearest along(previous on-route sample), or the
               earliest pass for the first
moving(i)    = Σ seconds between consecutive odometer-carrying samples up to i
               where 0 < seconds <= 10 s and the odometer advanced
readings     = the first on-route sample, then the first sample whose along
               reaches each further multiple of 100 m
predicted(a) = cumulativeSeconds interpolated at a, over each coordinate's
               along in the same frame
ahead(k)     = (predicted(along(k)) - predicted(along(k0)))
               - (moving(k) - moving(k0)),  k0 the first reading
~~~

Between two readings a sample's value is interpolated by sample index; before
the first and after the last there is none.

**Constants.** The 100 m spacing is `routeClockSpacingMetres` in
`internal/activity/routeclock.go`; the 40 m corridor is route matching's own,
and the 10 s gap is Recording gaps' `measure.DefaultMaxGap`. Passes are told
apart because a closed loop's finish lies within the corridor of its start, and
a ride's first seconds must not snap to the route's end.

**Choices.** The ride's clock is its moving time, never elapsed time, so a stop
is not read as the prediction being wrong. Both series are read at the same
place along the route rather than at the bicycle's odometer, which a detour or
GPS wander pulls away from the route. Only a ride that ran the way the route is
stored is compared: the prediction is not symmetric, so a ride the other way
round, or one whose direction could not be told, has none. A ride with no
odometer cannot tell moving from standing and has none either. A step longer
than a recording gap counts as a pause even where the odometer crept across it,
so a GPS dropout while riding reads as time gained. A comparison is served only
against the line the clock was read along, which the clock fingerprints.

**Source.** This service's own rule.

**Applied by.** `activity.ReadRouteClock` in `activity:derive`, on the pass that
matches a ride to its route, stored as `activity_route_match.route_clock`;
`activity.AheadOfPrediction` when the series is served, against the stage's
current `cumulativeSeconds`, so a refit prediction is never read against a
stale comparison.

**Status.** Covered by unit tests over synthetic rides; unvalidated against a
real ride's split times.

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

**Speed ceiling.** A recorded or derived speed reading above
`measure.MaxPlausibleSpeedKmh` (120 km/h) is dropped outright rather than
clamped or interpolated. Paved descents by strong riders peak around
100–120 km/h; a single sample far past that is an odometer or clock hiccup,
not a sprint. Applied where a ride's speed series is built
(`internal/sqlite/metrics.go` `speedFromRows`), before the ride's maximum
speed is worked out over what remains (`internal/activity/averages.go`
`meanAndPeak`, called from `RideSamples.Averages`).

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

**Outlook.** Where the last day served leaves the rider, on each scale,
and three weeks projected from there under three plans
(`internal/trainingload/outlook.go`):

~~~text
ramp(week)    = fitness(last) - fitness(last - 7 days)  [fitness(last - 7 days) is 0 where the timeline holds no such day]
habitual(day) = Σ load(last - 27 days .. last) / 28      [days before the timeline's first count as zero]
keep          = (1 - 1/42)^7
weekly(share) = 7 · fitness(last) · (1 + share - keep) / (1 - keep)
~~~

`weekly` is the load that, spread evenly over the next seven days, raises
fitness by `share`. It is derived by folding a constant daily load `d` from
`f0` for seven days (`f7 = d + (f0 - d)·keep`), setting `f7 = f0·(1+share)`,
and solving for `d`. The two figures reported are `weekly(0.03)` and
`weekly(0.08)` (`BuildingShareLow`, `BuildingShareHigh`). `habitualDailyLoad`
is the mean daily load over the last `HabitualDays` (28) days, the load a
rider has actually been carrying.

Each of the three plans then projects `OutlookDays` (21) days forward from
the last day, applying the same daily decay above under one constant daily
load: rest carries none, habitual carries `habitualDailyLoad` every day, and
build carries `habitualDailyLoad × BuildFactor` (1.2).

**Form bands.** The browser reads form as a percentage of fitness —
`form / fitness · 100`, nought where fitness is at or below 1 — against five
bands: Transition (≥ 20), Fresh (5 to 20), Grey zone (−10 to 5), Optimal
(−30 to −10), High risk (< −30). It reads the outlook's ramp the same way, as
a percentage of the fitness seven days earlier, against four bands:
Aggressive (≥ 10), Building (3 to 10), Holding (−3 to 3), Detraining (< −3).
Applied by `internal/webui/app/src/features/fitness/form.ts`.

**Heart-rate zones.** Five zones cut by four bounds, from a threshold rate
where the rider has entered one (0.81, 0.90, 0.94, 1.00 of threshold) or
from a maximum rate otherwise (0.60, 0.70, 0.80, 0.90 of maximum)
(`internal/trainingload/zones.go` `thresholdShares`, `maximumShares`,
`BoundsFrom`). The threshold scheme is preferred because a threshold is
measured and a maximum is often guessed. These are Friel's **cycling** cuts;
his running scheme cuts the same five zones at 0.85, 0.90, 0.95, 1.00 of
threshold instead, and that scheme is not this one — a bike app has no use
for run zones.

**Source.** TRIMP: Banister 1991. Heart-rate TSS and power TSS/IF/NP:
Coggan, in Allen and Coggan 2010. Heart-rate zones: Friel 2009.
Form-percentage bands: the convention intervals.icu uses. The weekly range,
ramp bands and the three plans: this service's own.

**Applied by.** `internal/trainingload/load.go` (`TRIMP`, `HeartRateTSS`,
`PowerLoad`), `internal/trainingload/zones.go` (`BoundsFrom`, `TimeInZones`,
`TimeAtHeartRate`), `internal/trainingload/fitness.go` (`Timeline`, `decay`),
`internal/trainingload/outlook.go` (`OutlookOf`),
`internal/webui/app/src/features/fitness/form.ts`.

**Status.** Validated: these are the figures a rider's own training-load
pages show today. The outlook and its bands are this service's own reading,
not validated against another application.

## Decoupling and heat drift

Two readings taken over a ride's own sensors, both from **measured power
only**. An estimated power is never fed to either figure (see §Estimated
power): comparing a ride against itself needs one consistent kind of number.

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

Enough in-band samples can still come from a series that covered only a
fraction of the ride's moving time: a reading is also withheld when the
heart-rate, power or temperature series behind it fell below
`trainingload.MinSeriesCoverage` (§Recorded activities), the same threshold
that withholds TRIMP, hrTSS and the power figures — otherwise that fraction
would be presented as a ride-wide reading.

Heart rate and temperature are paired by the second each was recorded at, which
is the resolution the records are stored at and so the only basis on which two
sensors are known to describe the same moment.

**Source.** Decoupling is Friel's Pw:Hr [14]. Heat drift has no single source:
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

**No threshold suggestion is read off the curve.** The curve's own point is the
raw best twenty minutes and the twenty-minute suggestion is 95% of it, so the
two are the same effort scaled differently rather than the same number. A
derivation runs only for a rider who has entered something, so the curve is
empty for a rider with no profile at all — who is exactly the rider a threshold
is suggested to. Every suggestion is therefore worked out from the stored
samples, the ramp-test estimate below included.

Where the suggestion is the twenty-minute estimate, it and the curve's point can
differ only while a ride's samples are stored and its derivation is still owed.
Where a ramp reading wins, they differ for good: the curve holds durations, and
a ramp reading is a protocol rather than a duration, so the curve carries no
point that could agree with it.

**Ramp test.** A rider who tests on a ramp never rides the twenty minutes the
estimate above scales, so a second estimate reads that protocol instead: 75% of
the ride's best minute, over a ride shaped like a ramp test — 20 to 40 minutes
of unbroken recorded span, holding a best minute 1.08 to 1.25 times its best
five (`internal/rider/ramp.go` `RampThresholdPower`). The suggestion is
whichever of the two estimates is higher, because each is a floor that only a
rider who performed that protocol reaches.

The band is closed at the bottom because a ramp is ridden until the next step
cannot be held, so its last minute necessarily stands above the five it closes.
Twenty watts a minute onto a peak of 250 to 400 puts the protocol's own ratio
between 1.11 and 1.19; a ride held flat sits at 1.00, and a steady half hour is
the opposite of a ramp however near threshold it was ridden. A gentler climb
than the protocol's own reads as no ramp and falls back to the twenty-minute
estimate, which is the safe direction to be wrong in.

The shape is deliberately loose and settles nothing on its own. The ratio bounds
what one ride can claim — an estimate is 75% of a minute that is itself at most
1.25 times the best five, so it never exceeds 94% of the five minutes it was
read over — but that bound is on the ride, not on the rider. An easy ride clears
it: sixteen minutes at 50 W, four at 100 and a closing minute at 125 is a ratio
of 1.19 and offers 94 W to a rider who never held 60.

The bound is on the ride and not on the rider, and there it stops. A rider
whose whole corpus is easy rides has no sustained twenty worth clearing, and
such a ride can be read as a ramp. Nothing inside the recorded data
distinguishes that case, and a rider whose riding says nothing about their
threshold is owed no better estimate by either protocol; the suggestion is
offered and never applied, so the rider is the check.

There is deliberately no test that the hardest minute is the ride's last: a ramp
is ridden to failure, but the file carries the cooldown after it, so a real
one's peak minute ends eleven to sixteen minutes before its last sample.

Both heart-rate suggestions read only a beating rate. An unpaired strap records
a nought that is present rather than absent, and averaging it in would offer a
rate no heart held — so a suggestion drops it, as the derivation path does
before it cleans anything (`internal/sqlite/metrics.go` `ActivityRideSamples`).

**Source.** Ramp test: 75% of best-minute power, the scaling Zwift's own ramp
test applies.

**Applied by.** `internal/activity/powercurve.go` (`PowerBests`),
`internal/sqlite/metrics.go` (`PowerCurve`).

**Status.** Unvalidated against another platform's curve. The per-ride bests are
covered by unit tests over synthetic streams with known bests.

## Nearest point on a line

**Definition.** The point on any of a set of polylines closest to a query
coordinate, and the distance to it in metres.

**Formula.** In symbols, per segment (`start`, `end`) of each line, in a local
planar frame centred on the query point:

~~~text
run  = end - start
t    = clamp(-(start·run) / |run|², 0, 1)
foot = start + t·run
~~~

the closest foot across every segment is the answer. The frame is the same
equirectangular projection `measure.SnapIndex` uses: longitude scaled by
cos(latitude) at the query point, both axes then converted to metres by
`EarthRadiusMetres`.

**Constants.** None beyond `EarthRadiusMetres` (see Spherical distance above).

**Source.** Standard point-to-segment projection, clamped to the segment; no
external source.

**Applied by.** `internal/measure/nearest.go` `NearestPoint`, called once per
`GET /v1/places/snap` request over the ways `surface.Snap`
(`internal/surface/snap.go`) reads near the point. Unlike `SnapIndex`, it
builds no grid and scans every segment it is given, which suits the handful of
ways near one point rather than a whole route's own samples.

**Status.** Unvalidated beyond unit tests over synthetic segments; no other
platform's figure to compare against.

## Walked stretches

**Definition.** The stretches of a routed plan where BRouter's own access
rules refuse a way to bicycles and price it for feet alone, scaled from the
engine's own line length onto the plan's normalised one.

**Formula.** In symbols:

~~~text
pushed(way)  = !bikeAccess(way.tags) && footAccess(way.tags)
scale        = totalMetres / engineMetres
window.start = way.startMetres * scale
window.end   = way.endMetres * scale
~~~

adjacent pushed ways merge into one window.

**Constants.** None; `bikeAccess`/`footAccess` read the way's own `bicycle`,
`bicycle_road`, `vehicle`, `foot` and `access` tags.

**Source.** BRouter's own `bikeaccess`/`footaccess` pricing: a stretch is
pushed exactly when the engine priced it as refused to bicycles and allowed to
feet. No external citation beyond BRouter's own profile rules.

**Applied by.** `internal/plan/pushing.go` `pushingOf`, called once per route
from `Service.Route` (`internal/plan/plan.go`) and stored with the plan on
create and replace, since only the engine's own answer at routing time says
which ways it ran along.

**Status.** Unvalidated beyond unit tests over synthetic way tag sets.

## Turn cue placement

**Definition.** Where along a routed plan each of the routing engine's turn
instructions falls, and so which course record a device reaches it on.

**Formula.** In symbols, for a turn on engine vertex `i`:

~~~text
engineAlong(i) = sum of haversine(p[k-1], p[k]) for k in 1..i over the engine's line
scale          = totalMetres / engineAlong(last)
cue.metres     = engineAlong(i) * scale
record         = the course record whose cumulative distance is nearest cue.metres
~~~

**Constants.** None. The engine's continue, keep, turn, U-turn and roundabout
commands are kept; leaving the route, beeline stretches and the end point are
not cues.

**Source.** BRouter's own voice hints (`timode`), one per vertex it judged a
decision point. No external citation.

**Applied by.** `internal/plan/cues.go` `cuesOf`, called once per route from
`Service.Route` and stored with the plan; `internal/fit/encoder.go`
`coursePoints` places each on its record when the plan's course carries cues.

**Status.** Unvalidated beyond unit tests over synthetic lines; whether a
device shows the cues it is given rather than its own is checked on a device.

## Waypoint progress

**Definition.** How far into a routed plan each waypoint falls, and the moving
time predicted to reach it, both read along the routed line rather than
between waypoints as placed.

**Formula.** In symbols, for each waypoint in order, searching only from the
last match onward:

~~~text
nearest        = index of the routed point nearest the waypoint, compared
                 over squared degrees with longitude scaled by cos(latitude)
distanceMetres = cumulative haversine distance to that point
movingSeconds  = the forward model's cumulative seconds at that point
~~~

`movingSeconds` comes from `ridemodel.Predict`'s forward model, the same one a
stage's moving time is predicted with, over the routed geometry's own steps:
`movingSeconds[i] = movingSeconds[i-1] + secondsPerKM·(span/1000) +
secondsPerAscentM·max(0, rise)`.

**Constants.** The calibrated `SecondsPerKM`/`SecondsPerAscentM` pair in force
at read time (see Ascent and descent above for how that pair is fitted).

**Source.** No external source; searching forward-only from the last match
assumes a routed line passes through its waypoints in the order they were
placed.

**Applied by.** `internal/plan/plan.go` `progressAt` for the index and
distance, and `ridemodel.Predict` for the moving time, both recomputed every
time a plan is routed or read — never stored — so a calibration change is
reflected the next time a plan is read.

**Status.** Unvalidated beyond unit tests over synthetic waypoints; the
forward model's own accuracy is discussed under Ascent and descent above.

## References

1. Sinnott, R. W. (1984) "Virtues of the Haversine", Sky and Telescope
   68(2):158.
2. Martin, J. C., Milliken, D. L., Cobb, J. E., McFadden, K. L., Coggan,
   A. R. (1998) "Validation of a Mathematical Model for Road Cycling
   Power", Journal of Applied Biomechanics 14(3):276-291.
3. Banister, E. W. (1991) "Modeling elite athletic performance" in
   Physiological Testing of Elite Athletes.
4. Allen, H. and Coggan, A. (2010) Training and Racing with a Power Meter,
   2nd ed.
5. The internal handover document at
   [docs/references/power-estimation-handover.md](../references/power-estimation-handover.md).
6. Strava "Elevation"
   <https://support.strava.com/en-us/articles/15401909-elevation>, accessed
   2026-09-07.
7. Strava "Elevation on Strava FAQs"
   <https://support.strava.com/hc/en-us/articles/115001294564-Elevation-on-Strava-FAQs>,
   accessed 2026-09-07.
8. Intervals.icu forum "Elevation gain off?"
   <https://forum.intervals.icu/t/elevation-gain-off/8463>, accessed
   2026-09-07.
9. Intervals.icu forum "Heartrate spikes now automatically fixed"
   <https://forum.intervals.icu/t/heartrate-spikes-now-automatically-fixed/174>,
   accessed 2026-09-07.
10. Rapaport, D. C. (2011) "Evaluating cumulative ascent: Mountain biking
    meets Mandelbrot", arXiv:1011.4778 [physics.data-an],
    <https://arxiv.org/abs/1011.4778>.
11. Menaspà, P., Haakonssen, E., Sharma, A., Clark, B. (2016) "Accuracy in
    measurement of elevation gain in road cycling", Journal of Science and
    Cycling 5(1):10–12, CC BY 3.0; reproduced as
    [menaspa-2016-elevation-gain-road-cycling.pdf](../references/menaspa-2016-elevation-gain-road-cycling.pdf).
12. Sánchez, R., Villena, M. (2020) "Comparative evaluation of wearable
    devices for measuring elevation gain in mountain physical activities",
    Proceedings of the Institution of Mechanical Engineers, Part P: Journal
    of Sports Engineering and Technology, doi:10.1177/1754337120918975.
13. US patent 5,058,427 (1991) "Accumulating altimeter with ascent/descent
    accumulation thresholds", <https://patents.justia.com/patent/5058427>.
14. Friel, J. "Aerobic Endurance Testing", josephfriel.com,
    <https://josephfriel.com/aerobic-endurance-testing/>; the Pw:Hr
    decoupling ratio as used by TrainingPeaks.
