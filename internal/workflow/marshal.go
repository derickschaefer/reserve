// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package workflow

import "gopkg.in/yaml.v3"

// Marshal returns a normalized YAML representation of the workflow document.
func Marshal(doc Document) ([]byte, error) {
	doc.Normalize()
	return yaml.Marshal(doc)
}
