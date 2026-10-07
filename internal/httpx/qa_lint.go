package httpx

import "os"

func qaLint() { os.Remove("x"); _ = os.Chmod("y", 0o777) }
