const aiSectionLinks = [
  { href: "#mcp-server", label: "MCP Server" },
  { href: "#internal-mcps", label: "Internal MCPs" },
  { href: "#rag-evaluation", label: "RAG Evaluation" },
  { href: "#debugging-assistant", label: "Debugging Assistant" },
  { href: "#connect-client", label: "Connect a Client" },
];

export function AISectionNav() {
  return (
    <nav
      aria-label="AI section navigation"
      className="mt-8 rounded-lg border border-foreground/10 bg-card p-3"
    >
      <div className="flex flex-wrap gap-2">
        {aiSectionLinks.map((link) => (
          <a
            key={link.href}
            href={link.href}
            className="rounded-md border border-foreground/10 px-3 py-2 text-sm text-muted-foreground transition-colors hover:border-primary/60 hover:text-foreground"
          >
            {link.label}
          </a>
        ))}
      </div>
    </nav>
  );
}
