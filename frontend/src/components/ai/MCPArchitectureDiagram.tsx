import { MermaidDiagram } from "@/components/MermaidDiagram";

const mcpArchitectureChart = `flowchart LR
  subgraph Clients ["External MCP clients"]
    direction TB
    CD[Claude Desktop]
    CX[Codex CLI]
    INS[MCP Inspector]
  end
  subgraph Server ["ai-service /mcp endpoint (Go)"]
    direction TB
    HTTP[HTTPS Streamable<br/>transport]
    AUTH{Bearer JWT?}
    REG[Tool registry<br/>12 tools]
    HTTP --> AUTH
    AUTH -->|valid token| REG
    AUTH -->|absent| REG
  end
  subgraph Backends ["Backends (in-cluster)"]
    direction TB
    EC[Ecommerce<br/>REST + gRPC]
    RAG[Python RAG bridge<br/>circuit breaker, OTel]
    QD[(Qdrant)]
    OLL[(Ollama)]
    RAG --> QD
    RAG --> OLL
  end
  CD -->|"public + auth-scoped<br/>tools"| HTTP
  CX --> HTTP
  INS -->|"discovery + invoke"| HTTP
  REG --> EC
  REG --> RAG`;

export function MCPArchitectureDiagram() {
  return <MermaidDiagram chart={mcpArchitectureChart} />;
}
