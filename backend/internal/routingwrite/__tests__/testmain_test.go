package routingwrite_test

import (
	"flag"
	"testing"
)

func TestMain(m *testing.M) {
	if parallel := flag.Lookup("test.parallel"); parallel != nil {
		_ = parallel.Value.Set("4")
	}
	m.Run()
}
