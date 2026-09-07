> External handover document, reproduced verbatim for citation. Validated on
> one ride only; it is not normative for this service.

# Estimating cycling power from a FIT file with no power meter

Handover spec. Reference implementation: `estimate_power.py`.

Validated against one real ride (69.4 km, flat, Wahoo ELEMNT BOLT, gravel bike).
Everything marked **VALIDATED** was tested; everything marked **UNRESOLVED** was
tested and failed, or remains open. Do not silently promote the latter.

---

## 1. Inputs

Read per-second `record` messages. Check what is actually present rather than
assuming — devices write more than users expect.

| Field | Use | Notes |
|---|---|---|
| `distance` | grade denominator, speed fallback | prefer wheel-sensor over GPS-integrated |
| `enhanced_altitude` / `altitude` | grade | barometric strongly preferred |
| `enhanced_speed` / `speed` | main driver | fall back to `d(distance)/dt` |
| `cadence` | coasting gate | see §5 |
| `heart_rate` | wind fitting, validation | never a direct power predictor |
| `temperature` | air density | default 20 °C if absent |
| `position_lat/long` | wind bearing | without it, wind is a scalar headwind only |

Also read `session` for `total_ascent`, `threshold_power`, `total_calories` —
all useful as cross-checks.

---

## 2. Core model

Martin et al. (1998), *Validation of a Mathematical Model for Road Cycling
Power*. J Appl Biomech 14(3):276-291.

```
P = [ F_gravity + F_rolling + F_aero + F_inertia ] · v / η

F_gravity  = m · g · sin(θ)
F_rolling  = Crr · m · g · cos(θ)
F_aero     = ½ · ρ · CdA · |v + w| · (v + w)      # signed, not squared
F_inertia  = (m + m_rot) · a
```

- `θ = arctan(grade)`, grade dimensionless
- `g = 9.80665`
- `η = 0.977` drivetrain efficiency
- `m` = **total system mass**: rider + bike + kit + bottles. Not body weight.
- `m_rot = 1.5 kg` equivalent linear mass for wheel rotational inertia
- `w` = headwind component, positive into the wind (§6)

**Use `|v+w|·(v+w)`, not `(v+w)²`.** Squaring loses the sign, so a tailwind
faster than the rider produces a spurious positive drag force instead of a push.

### Air density

```
p   = 101325 · (1 - 2.25577e-5 · h)^5.25588        # h in metres
ρ   = p / (287.058 · (T + 273.15))                 # T in °C
```

### Reference coefficients

| CdA (m²) | Position |
|---|---|
| 0.20–0.25 | TT / aerobars |
| 0.30 | road, drops |
| 0.36 | road, hoods |
| 0.40 | gravel, hoods |
| 0.45 | upright |

| Crr | Surface |
|---|---|
| 0.004–0.005 | road slicks on asphalt |
| 0.006 | slicks, rougher tarmac |
| 0.009 | gravel tyres on tarmac |
| 0.012–0.018 | loose gravel |

---

## 3. Grade — the critical step

**Differentiate altitude against DISTANCE, not time**, on a fixed-distance grid.
This keeps the smoothing window a constant number of metres regardless of speed,
which matters through stops and climbs.

```
grid      = arange(0, total_distance, 20 m)
alt_grid  = interp(grid, distance, altitude)
alt_grid  = savgol_filter(alt_grid, window=15, polyorder=2)   # 15 × 20 m = 300 m
grade     = interp(distance, grid, gradient(alt_grid, 20 m))
```

### Window length is derivable, not a taste parameter

Barometric altimeters quantise (0.2 m on a Wahoo Bolt). One quantisation step
over one second of travel is a large grade error:

```
0.2 m / 6.8 m per sample = 2.9% grade per step
```

At 95 kg and 6.9 m/s, **1% of grade is 64 W**. So 2.9% of quantisation noise is
~185 W per sample. Required window:

```
window ≥ altitude_resolution / target_grade_precision
0.2 / 0.002 = 100 m           # for 0.2% grade precision
```

300 m leaves margin. **This single step is the difference between a usable
estimate and noise** — see §9.

---

## 4. Speed and acceleration

Smooth speed before differentiating, or the inertia term amplifies sensor noise
into large fake power spikes:

```
v = savgol_filter(raw_speed, window=11, polyorder=2)
a = gradient(v, t)
```

Sensitivity is low (±2 W across window 1–31), so this is not a tuning knob.

---

## 5. Cadence gate vs. clipping — keep these separate

Two different operations. Conflating them hides how noisy the estimate is.

```
watts = force · v / η
watts = where(cadence == 0, 0, watts)     # PHYSICS: no pedalling, no power
watts = clip(watts, 0, 1500)              # NUMERICAL PATCH for residual noise
```

The clip must be **last** and its effect must be **measured** (§7). Clipping a
noisy symmetric signal at zero is asymmetric: you keep upward excursions and
discard downward ones, so the *mean* ratchets up. This is the single most
important failure mode in this problem domain.

---

## 6. Wind

`w = wind_speed · cos(radians(wind_from_deg − bearing))`

where `bearing` is heading in degrees from north, derived from GPS:

```
dx = gradient(lon) · cos(mean_lat) · 111320
dy = gradient(lat) · 110540
bearing = degrees(arctan2(dx, dy)) mod 360
```

### Fit the wind. Do not inject a forecast. **VALIDATED**

Regional forecasts are ~13 km resolution and routinely wrong at road level. On
the test ride, injecting the forecast (2.7 m/s easterly) made every diagnostic
*worse*: HR residual 45.1 W vs 23.6 W fitted, and it manufactured a fake −35%
decoupling and a fake 268 W 20-minute effort at an average HR of 120.

Objective — grid search wind speed and direction minimising the RMS error of a
linear HR→power fit over 5-minute blocks:

```
blocks = group by (t // 300)
fit    P_block = α · HR_block + β    by least squares
score  = RMS(residual)
```

HR is a poor instantaneous power predictor but a good arbiter of whether the
model is internally coherent over multi-minute blocks.

**Recovery test passed**: synthetic HR generated from known winds of 0.0, 1.5,
2.7 and 4.0 m/s was recovered to within 0.5 m/s and 15° in all four cases. The
fitter is not biased toward calm.

### Wind cancels on loops, but not within them

On a route with headings spread across all quadrants, total power is nearly
insensitive to wind *direction* — rotating a 2.6 m/s wind through all eight
compass points moved the ride average by 5 W (155–160 W). Wind adds a consistent
~+5% versus still air because drag is convex, but direction barely matters for
the total.

**Direction matters enormously for the within-ride distribution** — NP, TSS,
power-duration curve, decoupling. Do not conclude from loop-cancellation that
wind can be ignored.

---

## 7. Quality diagnostics — mandatory, gate outputs on these

These are self-diagnosing. No external validation needed.

| Metric | Target | Real power meter | Strava on the test ride |
|---|---|---|---|
| lag-1 autocorrelation | > +0.80 | ~+0.90 | **−0.22** |
| mean \|ΔP\| per second | < 40 W | 10–20 W | **196 W** |
| clipping bias | < ~8 W | n/a | **+20 to +34 W** |

```
clip_bias = mean(clipped) − mean(unclipped)
```

Negative autocorrelation is the cheapest lie detector available: real power is
autocorrelated at ~+0.9 across one second because legs have inertia. A signal
that alternates sample-to-sample is a differentiated quantity, not a
physiological one.

**If the checks fail, report average power and kJ only.** Normalized Power
raises to the fourth; the power-duration curve takes maxima. Both amplify noise
superlinearly. On the test ride the 5 s figure (761 W) was pure artifact while
the 20-minute figure was sound.

> Threshold note: the reference implementation uses `clip_bias < 5.0`, which the
> test ride straddles (5.0 W at 95 kg passes, 5.4 W at 100 kg fails). A 0.4 W
> difference is not a meaningful change in data quality. Loosen to ~8 W or make
> it graded rather than pass/fail.

---

## 8. What is and is not identifiable

**CdA is NOT identifiable from the HR objective. UNRESOLVED — do not attempt.**
Recovery test: synthetic HR generated from known CdA of 0.34, 0.40 and 0.46 all
collapsed to the bottom of the scan range (0.28–0.30). Wind interacts with
bearing and has a directional signature; CdA does not, so the objective simply
shrinks it. Same applies to Crr.

CdA and Crr must come from lookup tables (§2), a power-meter calibration ride,
or the grade-residual method below.

### Grade-residual method for mass — needs terrain

Gravity scales with mass and grade; aero scales with v² and is grade-independent.
So regress power on HR, then examine the residual against grade:

- positive slope → mass set too low
- negative slope → mass set too high
- zero crossing → estimate

**Requires hills.** On the flat test ride (5th–95th percentile grade span 4.5%)
the slope moved only 2.8 W per % grade across a 35 kg range, and the crossing
was also sensitive to CdA — the two absorb the same error. Warn and suppress
below ~4% grade span. This becomes a genuine second equation only with sustained
climbs above ~6%.

---

## 9. Reference failure mode: Strava's `watts_calc`

Worth encoding as a regression test. On the same ride Strava reported 183 W
against this implementation's 157 W.

Diagnosis, from Strava's own CSV export:

1. It **kept** the good barometric altitude (range identical to the FIT file) —
   the input data was fine.
2. It differentiated that 0.2 m-quantised altitude over ~1 s baselines. Its
   `altitude_smooth` column is barely distinguishable from raw. Resulting
   `grade_smooth` σ = 2.0%, max 19% — where the head unit's own barometric grade
   maxed at 7.5%.
3. Its power channel has **zero negative values and 19.4% exact zeros** — hard
   clipping.
4. It ignores cadence: while cadence was zero, its power averaged **90 W**.

Simulating this — adding white noise to a clean estimate and clipping at zero —
reproduces its published statistics at σ ≈ 150–200 W, implying a clipping bias
of +20 to +34 W. Subtracting that from 183 W gives 149–163 W, bracketing this
implementation's independent 157 W.

**The two models do not disagree about physics. The entire gap is the clipping
artifact.** Tell: Strava's *average* (183 W) nearly equals this implementation's
*normalized* power (185 W). A spiky signal's mean drifts toward what a clean
signal's NP would be.

---

## 10. Cross-checks to implement

1. **Energy vs. device calories.** `kJ / 4.184 / 0.22` ≈ kcal at 22% gross
   efficiency. Test ride: 1587 kJ → 1738 kcal vs the device's 1773. Note the
   device figure is itself usually HR-derived, so this is not fully independent.
2. **Plausibility vs. FTP.** Average power as a fraction of FTP should be
   consistent with average HR. The test ride's Strava figure implied 74% of FTP
   for 2h49 at an average HR of 120 having peaked at 162 — tempo power at
   endurance heart rate, which is the tell.
3. **Compare eTSS against HR-based TSS.** A large divergence indicates one of
   the two is wrong. On the test ride, both power estimates gave TSS ~150 while
   HR-based tTSS gave 87 — unresolved, and a reason not to over-trust either.

---

## 11. UNRESOLVED: time-varying wind

A two-segment wind fit (independent vector each half) substantially outperforms
a single vector on the test ride:

| Model | Fitted wind | HR residual | Avg power |
|---|---|---|---|
| 1 segment | 0.5 m/s @ 90° | 23.7 W | 157 W |
| 1 segment + drift term | 1.0 m/s @ 60° | 19.0 W | 158 W |
| 2 segments | 2.0 @ 210°, then 3.0 @ 60° | 14.1 W | 126 W |
| 2 segments + drift | 1.0 @ 210°, then 3.0 @ 60° | 12.2 W | 131 W |

Evidence for: null test showed the two-segment model can only invent ~1 m/s from
nothing (13% residual cut); held-out validation gave 14.2 W out-of-sample vs
21.1 W for the single vector; it survives a linear cardiac-drift control; and
the recovered second-half vector (3.0 m/s from 60°) independently matches the
afternoon forecast (2.7 m/s easterly) it never saw.

Evidence against: it breaks the energy cross-check (131 W → ~1450 kcal vs the
device's 1773, an 18% miss, where the single-vector answer matched to 3%). The
held-out blocks were alternating 5-minute windows, which are temporally adjacent
and therefore correlated — weak cross-validation that flatters the more flexible
model. And the estimate fell monotonically as wind parameters were added
(157 → 131 W), which is the signature of a model absorbing structure it should
not.

**Recommendation:** default to the single vector. Expose two-segment behind a
flag. Report the honest band as **130–160 W** for this ride rather than the
overconfident 157 ± 12%. Do not add a third segment — parameters are multiplying
faster than evidence.

---

## 12. Expected accuracy

- Steady climbing, calibrated coefficients: ±5–10%
- Rolling terrain, fitted wind: ±15%
- Flat rides: aero-dominated, and CdA is unidentifiable, so the estimate is only
  as good as the assumed coefficients
- NP and the power-duration curve: only when §7 passes
- 5-second peaks: never trust these

The honest use case is **relative comparison** — same route, same bike, same
position, tracking fitness over time. Absolute watts for zone prescription is a
stretch. One ride with a borrowed power meter pins CdA and Crr directly and
removes most of this uncertainty.

---

## 13. References

- Martin et al. (1998), *Validation of a Mathematical Model for Road Cycling
  Power*, J Appl Biomech 14(3):276-291 — canonical formulation, R² = .97,
  SE 2.7 W with *measured* CdA, Crr and wind.
- Danek et al., arXiv:2005.04229 and arXiv:2005.04480 — least-squares estimation
  of CdA, Crr and drivetrain loss.
- Parsing: `fitdecode` (used here) or `python-fitparse`.
