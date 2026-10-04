//go:build race

package service

// raceEnabled reports whether the binary was built with -race. Starting a real
// sing-box Box trips a data race of sing-box's own (see core/race_on_test.go),
// so the tests here that need a running core skip under -race and run
// normally otherwise.
const raceEnabled = true
