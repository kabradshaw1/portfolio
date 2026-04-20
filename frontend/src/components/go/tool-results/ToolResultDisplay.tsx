"use client";

import { type ToolDisplay, CATALOG_TOOLS, RAG_TOOLS } from "./types";
import { ProductListResult } from "./ProductListResult";
import { ProductCardResult } from "./ProductCardResult";
import { CartResult } from "./CartResult";
import { CartItemResult } from "./CartItemResult";
import { SearchResultsResult } from "./SearchResultsResult";
import { RagAnswerResult } from "./RagAnswerResult";
import { OrderListResult } from "./OrderListResult";
import { OrderCardResult } from "./OrderCardResult";
import { InventoryResult } from "./InventoryResult";
import { CollectionsResult } from "./CollectionsResult";
import { ReturnResult } from "./ReturnResult";

type Props = {
  toolName: string;
  display: unknown;
};

function isToolDisplay(d: unknown): d is ToolDisplay {
  return typeof d === "object" && d !== null && "kind" in d;
}

function SourceLabel({ toolName }: { toolName: string }) {
  if (CATALOG_TOOLS.has(toolName)) {
    return (
      <div className="mb-1 flex items-center gap-1 text-[10px] font-semibold uppercase text-blue-500">
        <span aria-hidden>📦</span> Catalog Search
      </div>
    );
  }
  if (RAG_TOOLS.has(toolName)) {
    return (
      <div className="mb-1 flex items-center gap-1 text-[10px] font-semibold uppercase text-green-500">
        <span aria-hidden>📄</span> Product Knowledge
      </div>
    );
  }
  return null;
}

export function ToolResultDisplay({ toolName, display }: Props) {
  if (!isToolDisplay(display)) {
    return (
      <div className="rounded-lg border border-dashed p-3">
        <div className="mb-1 text-xs font-semibold text-muted-foreground">
          {toolName}
        </div>
        <pre className="max-h-48 overflow-auto text-xs text-muted-foreground">
          {JSON.stringify(display, null, 2)}
        </pre>
      </div>
    );
  }

  const borderClass = CATALOG_TOOLS.has(toolName)
    ? "border-l-blue-500"
    : RAG_TOOLS.has(toolName)
      ? "border-l-green-500"
      : "border-l-muted";

  return (
    <div className={`rounded-lg border-l-[3px] bg-muted/50 ${borderClass}`}>
      <div className="p-3">
        <SourceLabel toolName={toolName} />
        <DisplayContent display={display} />
      </div>
    </div>
  );
}

function DisplayContent({ display }: { display: ToolDisplay }) {
  switch (display.kind) {
    case "product_list":
      return <ProductListResult products={display.products} />;
    case "product_card":
      return <ProductCardResult product={display.product} />;
    case "cart":
      return <CartResult cart={display.cart} />;
    case "cart_item":
      return <CartItemResult item={display.item} />;
    case "search_results":
      return <SearchResultsResult results={display.results} />;
    case "rag_answer":
      return <RagAnswerResult answer={display.answer} sources={display.sources} />;
    case "order_list":
      return <OrderListResult orders={display.orders} />;
    case "order_card":
      return <OrderCardResult order={display.order} />;
    case "inventory":
      return <InventoryResult productId={display.product_id} stock={display.stock} inStock={display.in_stock} />;
    case "collections_list":
      return <CollectionsResult collections={display.collections} />;
    case "return_confirmation":
      return <ReturnResult ret={display.return} />;
    default: {
      const _exhaustive: never = display;
      return (
        <pre className="text-xs text-muted-foreground">
          {JSON.stringify(_exhaustive, null, 2)}
        </pre>
      );
    }
  }
}
