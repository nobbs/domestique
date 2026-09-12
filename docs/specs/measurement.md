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

**Status.** Validated against the operator's own trainer power at matched heart rate: ride means within 6% on rides held out from the check, which `dev/levelstudy` reproduces from a state snapshot.

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

**Applied by.** `internal/trainingload/load.go` (`TRIMP`, `HeartRateTSS`,
`PowerLoad`), `internal/trainingload/zones.go` (`BoundsFrom`, `TimeInZones`),
`internal/trainingload/fitness.go` (`Timeline`, `decay`).

**Status.** Validated: these are the figures a rider's own training-load
pages show today.

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

**No threshold suggestion is read off the curve**, though the twenty-minute one
and the curve's own point are both 95% of the same best twenty minutes. A
derivation runs only for a rider who has entered something, so the curve is
empty for a rider with no profile at all — who is exactly the rider a threshold
is suggested to. Every suggestion is therefore worked out from the stored
samples, the ramp-test estimate below included. The two can differ only while a
ride's samples are stored and its derivation is still owed.

**Ramp test.** A rider who tests on a ramp never rides the twenty minutes the
estimate above scales, so a second estimate reads that protocol instead: 75% of
the ride's best minute, over a ride shaped like a ramp test — 20 to 40 minutes
of recorded span, holding a best minute within 1.25 times its best five
(`internal/rider/ramp.go` `RampThresholdPower`). The suggestion is whichever of
the two estimates is higher, because each is a floor that only a rider who
performed that protocol reaches.

The shape is deliberately loose, and the ratio is what makes that safe rather
than the shape: an estimate is 75% of a minute that is itself at most 1.25 times
the best five, so it can never exceed 94% of a rider's best five-minute power. A
steady ride mistaken for a ramp therefore estimates below the threshold it is
compared against and loses. There is deliberately no test that the hardest
minute is the ride's last: a ramp is ridden to failure, but the file carries the
cooldown after it, so a real one's peak minute ends eleven to sixteen minutes
before its last sample.

**Source.** Ramp test: 75% of best-minute power, the scaling Zwift's own ramp
test applies.

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
