package resources

// NewSchemaResource returns schema://ecommerce backed by the markdown file
// at path. The content is loaded once per process.
func NewSchemaResource(path string) (Resource, error) {
	return newFileBackedResource(
		"schema://ecommerce",
		"Ecommerce data model",
		"Sanitized ER summary of the ecommerce databases.",
		"text/markdown",
		path,
	)
}
