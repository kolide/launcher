package localserver

import (
	"context"
	"fmt"
	"testing"
)

func Test_generateSelfSignedCert(t *testing.T) {
	t.Parallel()

	fmt.Println(generateSelfSignedCert(context.TODO()))
}
