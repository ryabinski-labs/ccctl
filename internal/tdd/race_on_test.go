//go:build race

package tdd

// raceEnabled reports a -race build; timing budgets do not apply there.
const raceEnabled = true
