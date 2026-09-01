// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import "github.com/derickschaefer/reserve/internal/workflow"

func workflowDocumentBytes(doc workflow.Document) ([]byte, error) {
	return workflow.Marshal(doc)
}
