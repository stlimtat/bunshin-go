package credentials

import "os"

// envGet wraps os.Getenv so it can be swapped in tests.
var envGet = os.Getenv
