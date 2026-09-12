package measure

// kilojoulesPerKilocalorie and grossCyclingEfficiency are the cross-check in
// docs/references/power-estimation-handover.md §10: kJ / 4.184 / 0.22 ≈ kcal
// at 22% gross efficiency.
const (
	kilojoulesPerKilocalorie = 4.184
	grossCyclingEfficiency   = 0.22
)

// EstimatedCalories is the energy a ride at meanWatts over seconds cost the
// rider, worked out at a fixed gross efficiency. A comparison figure only:
// never mixed with a device's own reported calories, which is itself usually
// heart-rate-derived rather than an independent measurement.
func EstimatedCalories(meanWatts, seconds float64) float64 {
	return meanWatts * seconds / 1000 / kilojoulesPerKilocalorie / grossCyclingEfficiency
}
