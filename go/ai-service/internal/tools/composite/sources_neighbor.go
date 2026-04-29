package composite

import "context"

// NopNeighborSearch returns no results — used until product embeddings
// exist in Qdrant. Products are currently stored only in Postgres
// (product-service) and are not indexed in the vector store, so
// embedding-based nearest-neighbor lookup is not possible.
//
// The recommend_with_rationale tool degrades gracefully with empty neighbor
// results: when signals exist but have no embeddings (v1 state), the
// response is {products:nil, query_embedding_source:"no_embeddings"}.
// When product embeddings are added to Qdrant, swap this with a
// QdrantNeighborSearch that queries the product-embeddings collection.
type NopNeighborSearch struct{}

// Nearest always returns nil, nil.
func (NopNeighborSearch) Nearest(_ context.Context, _ []float32, _ int, _ []string, _ string) ([]NeighborResult, error) {
	return nil, nil
}
