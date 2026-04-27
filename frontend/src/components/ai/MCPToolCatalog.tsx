import { MermaidDiagram } from "@/components/MermaidDiagram";

const toolCatalogChart = `flowchart LR
  AGENT((MCP client<br/>or in-app agent))
  subgraph Catalog ["Catalog (public)"]
    T1[search_products<br/>query + max_price]
    T2[get_product<br/>full details by ID]
    T3[check_inventory<br/>stock count]
  end
  subgraph Orders ["Orders (auth-scoped)"]
    T4[list_orders<br/>last 20 orders]
    T5[get_order<br/>single order detail]
    T6[summarize_orders<br/>LLM-generated summary]
  end
  subgraph CartReturns ["Cart & Returns (auth-scoped)"]
    T7[view_cart<br/>items + total]
    T8[add_to_cart<br/>product + quantity]
    T9[initiate_return<br/>order item + reason]
  end
  subgraph Knowledge ["Knowledge Base (public, RAG)"]
    T10[search_documents<br/>semantic search + sources]
    T11[ask_document<br/>natural-language Q&A]
    T12[list_collections<br/>vector store inventory]
  end
  AGENT --> Catalog
  AGENT --> Orders
  AGENT --> CartReturns
  AGENT --> Knowledge
  X[place_order<br/>deliberately excluded]:::disabled
  AGENT -.-x X
  classDef disabled stroke-dasharray: 5 5,opacity:0.5`;

export function MCPToolCatalog() {
  return (
    <div>
      <h3 className="mt-10 text-xl font-semibold">Tool Catalog</h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The MCP server exposes twelve tools across four domains. Catalog and
        knowledge-base tools are public; order, cart, and return tools require a
        Bearer JWT. Knowledge-base tools call the Python RAG pipeline through a
        circuit-breaker HTTP bridge with a 30-second timeout. Checkout
        (<code>place_order</code>) is deliberately excluded &mdash; the agent
        can advise but not transact.
      </p>
      <div className="mt-6">
        <MermaidDiagram chart={toolCatalogChart} />
      </div>
    </div>
  );
}
