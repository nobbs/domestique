// Command revision prints what internal/build reports, so a test can prove that
// the documented -X symbol is the one the code actually reads. It lives under
// testdata so it is not part of the module's own package list.
package main

import (
	"fmt"
	"os"

	"github.com/nobbs/domestique/internal/build"
)

func main() {
	info := build.Current(os.Getenv("IMAGE_REFERENCE"))
	fmt.Printf("%s %s\n", info.Revision, info.ImageDigest)
}
