package cola

import (
	"sync"
)

var (
	configs sync.Map // Global config
)

// Config gets config by key
func Config(key string) (interface{}, bool) {
	return configs.Load(k)
}
