// Types for the display payloads returned by the Go ai-service.
// Each tool result has a `kind` discriminator field.

export type ProductItem = {
  id: string;
  name: string;
  price: number; // cents
  stock?: number;
  category?: string;
};

export type CartItem = {
  id: string;
  product_id: string;
  product_name: string;
  product_price: number; // cents
  quantity: number;
};

export type OrderSummary = {
  id: string;
  status: string;
  total: number; // cents
  created_at: string;
};

export type SearchChunk = {
  text: string;
  filename: string;
  page_number: number;
  score: number;
};

export type RagSource = {
  file: string;
  page: number;
};

// Discriminated union of all display payload shapes
export type ToolDisplay =
  | { kind: "product_list"; products: ProductItem[] }
  | { kind: "product_card"; product: ProductItem }
  | { kind: "cart"; cart: { items: CartItem[]; total: number } }
  | { kind: "cart_item"; item: CartItem }
  | { kind: "search_results"; results: SearchChunk[] }
  | { kind: "rag_answer"; answer: string; sources: RagSource[] }
  | { kind: "order_list"; orders: OrderSummary[] }
  | { kind: "order_card"; order: OrderSummary }
  | { kind: "inventory"; product_id: string; stock: number; in_stock: boolean }
  | { kind: "collections_list"; collections: { name: string; point_count: number }[] }
  | { kind: "return_confirmation"; return: { id: string; order_id: string; status: string; reason: string } };

// Tools that query the product catalog (database)
export const CATALOG_TOOLS = new Set([
  "search_products",
  "get_product",
  "check_inventory",
  "view_cart",
  "add_to_cart",
  "list_orders",
  "get_order",
  "summarize_orders",
  "initiate_return",
]);

// Tools that query RAG document knowledge
export const RAG_TOOLS = new Set([
  "search_documents",
  "ask_document",
  "list_collections",
]);
