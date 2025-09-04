package orchestration

import "os"

func mapGetenv(key string) string { return os.Getenv(key) }

