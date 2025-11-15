package mcpsdk

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// emptySchema describes a function with no parameters.
// Many clients require explicit properties: {} and required: [].
var emptySchema jsonschema.Schema

func init() {
	if err := json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {},
		"required": [],
		"additionalProperties": false
	}`), &emptySchema); err != nil {
		panic(fmt.Errorf("failed to create empty input schema: %w", err))
	}
}
