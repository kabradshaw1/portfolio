# AI Tool Schema Guidance Design

## Context

GitHub issue 264 asks to tighten the Go AI service's schema-backed tool
descriptions and parameter schemas. Better tool contracts should improve model
tool selection, reduce unnecessary calls, and avoid unsafe user-boundary
assumptions.

The target surface is `go/ai-service/internal/tools/`, including RAG, catalog,
order, cart, return, and composite tools. Each tool currently owns its
`Name()`, `Description()`, and `Schema()`, and the registry passes those values
unchanged to OpenAI-compatible, Ollama, Anthropic, and MCP adapters.

## Goals

- Make advertised tool descriptions clear enough for model tool selection.
- Add concise use-when and do-not-use-when guidance where it materially changes
  model behavior.
- Clarify whether each tool is public/read-only or scoped to the authenticated
  user.
- Keep schemas compatible with the existing provider adapters.
- Add focused tests for important schema, description, and auth-boundary
  contract details.

## Non-Goals

- Do not introduce a new schema-generation framework.
- Do not rewrite the tool registry or provider adapters.
- Do not broaden authorization beyond the minimal runtime fixes required to
  make advertised tool contracts truthful.
- Do not add provider-specific schema features that might work for one backend
  but fail for another.

## Approach

Use a contract cleanup plus minimal boundary fixes approach.

Descriptions and schemas will be updated in place near each tool definition.
The implementation will add small behavior changes only when needed to make the
model-facing contract true. This avoids advertising authenticated-user
boundaries that runtime code does not enforce.

## Tool Boundary Model

Public and read-only tools:

- `search_products`
- `get_product`
- `check_inventory`
- `compare_products`
- `search_documents`
- `ask_document`
- `list_collections`

These descriptions should say the tools are for public catalog or document
knowledge. They should not be used for account-specific order history, cart
contents, returns, or other authenticated-user state. Catalog tools may still
emit existing analytics events, but they do not require authentication.

Authenticated-user tools:

- `list_orders`
- `get_order`
- `summarize_orders`
- `view_cart`
- `add_to_cart`
- `initiate_return`

These descriptions should say the tools operate only for the authenticated
user. Schemas should not expose `user_id`, and descriptions should tell the
model not to ask for or invent another user id.

Composite user tools:

- `recommend_with_rationale` should use the authenticated user as its user
  boundary. The advertised schema should not invite arbitrary user lookup.
  Either remove `user_id` from the schema or keep backwards-compatible parsing
  while rejecting mismatches.
- `investigate_my_order` should keep `order_id` as the only model-facing
  argument. If fetched evidence contains an order owner, the tool should enforce
  that it matches the authenticated user. If ownership cannot be established,
  customer-facing authenticated use should fail closed rather than return
  another user's operational evidence.

## Description Style

Each changed description should be concise and should include:

- The concrete capability.
- `Use when...` guidance when it helps distinguish similar tools.
- `Do not use when...` guidance when it prevents unsafe or wasteful calls.
- Auth language only when runtime code enforces that boundary.
- Units or limits where they affect model input selection.

Descriptions should not become long policy prompts. The service advertises
tools on every LLM turn, so clarity and brevity both matter.

## Schema Style

Schemas remain plain JSON Schema objects stored as `json.RawMessage`.

Allowed schema features for this enhancement:

- `type`
- `properties`
- `required`
- property `description`
- `enum`
- `minimum`
- `minItems`
- `maxItems`

Important properties should gain descriptions where missing, especially:

- `product_id`
- `order_id`
- `item_ids`
- `reason`
- `qty`
- `limit`
- `period`
- `category`
- RAG `query`, `question`, and `collection`

Units should be explicit. For example, `max_price` is in dollars, while product
prices and order totals returned by the service are in cents.

## Error Handling

Existing argument validation errors remain tool errors.

Authenticated-user tools continue to reject empty `userID` with an
authenticated-user-required error. Composite tools whose descriptions become
authenticated-user scoped should follow the same pattern.

Cross-user access should fail closed where the tool can detect it. For
`recommend_with_rationale`, that means preventing arbitrary `user_id` use in the
model-facing contract. For `investigate_my_order`, that means rejecting an order
whose fetched owner does not match the authenticated user, and rejecting partial
evidence when ownership cannot be established for customer-facing use.

## Testing

Add focused tests rather than brittle full-copy assertions.

Contract tests should verify:

- All targeted schemas are valid JSON objects.
- Important properties include descriptions.
- Required fields remain present.
- Authenticated-user tools do not advertise a `user_id` parameter.
- Key descriptions include the intended use and do-not-use guidance.

Behavior tests should verify:

- `recommend_with_rationale` rejects missing auth and does not allow arbitrary
  cross-user access through `user_id`.
- `investigate_my_order` rejects missing auth, owner mismatch, and missing
  ownership evidence when the contract says authenticated-user only.

Run targeted Go tests for the touched packages first. Before implementation is
committed, run `make preflight-go`.
