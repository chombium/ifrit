package ifrit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestIfrit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ifrit Suite")
}
