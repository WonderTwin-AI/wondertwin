package seed

import (
	_ "embed"
	"encoding/json"
	"slices"
	"sync"
)

//go:embed processors.json
var processorsJSON []byte

var (
	processorsOnce sync.Once
	processors     []string
)

// IsProcessor reports whether name is a processor Plaid documents for
// /processor/token/create. The list is recorded in processors.json.
func IsProcessor(name string) bool {
	processorsOnce.Do(func() {
		var f struct {
			Processors []string `json:"processors"`
		}
		if err := json.Unmarshal(processorsJSON, &f); err != nil {
			panic("seed: processors.json: " + err.Error())
		}
		processors = f.Processors
	})
	return slices.Contains(processors, name)
}
