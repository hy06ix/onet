package onet

import (
	"testing"

	"github.com/hy06ix/onet/log"
)

// To avoid setting up testing-verbosity in all tests
func TestMain(m *testing.M) {
	log.MainTest(m)
}
