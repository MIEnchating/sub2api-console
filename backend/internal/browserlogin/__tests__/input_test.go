package browserlogin_test

import (
	"math"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestInputRejectsCommandsOutsideOperatorSurface(t *testing.T) {
	for _, input := range []browserlogin.Input{{Kind: "evaluate", Text: "document.cookie"}, {Kind: "click", X: -1}, {Kind: "click", X: math.NaN()}, {Kind: "click", X: browserlogin.Width}, {Kind: "scroll", Delta: 2000}, {Kind: "key", Key: "F12"}, {Kind: "text"}} {
		if input.Validate() == nil {
			t.Fatalf("accepted invalid input: %#v", input)
		}
	}
}
func TestInputAcceptsClickTextScrollAndKeyboardNavigation(t *testing.T) {
	for _, input := range []browserlogin.Input{{Kind: "click", X: 100, Y: 200}, {Kind: "text", Text: "人工输入"}, {Kind: "scroll", Delta: 400}, {Kind: "key", Key: "Tab", Shift: true}, {Kind: "key", Key: "Enter"}} {
		if err := input.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
