package runbooks

import (
	"embed"
	"fmt"
)

//go:embed *.md
var files embed.FS

func Read(name string) (string, error) {
	data, err := files.ReadFile(name + ".md")
	if err != nil {
		return "", fmt.Errorf("read runbook %q: %w", name, err)
	}
	return string(data), nil
}
