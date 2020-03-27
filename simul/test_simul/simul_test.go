package main

import (
	"testing"

	"github.com/csanti/onet/simul"
)

func TestSimulation(t *testing.T) {
	simul.Start("count.toml")
}
