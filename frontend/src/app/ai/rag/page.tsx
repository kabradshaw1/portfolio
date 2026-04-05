"use client";

import { useState, useCallback, useEffect } from "react";
import Link from "next/link";
import { ChatWindow, Message, Source } from "@/components/ChatWindow";
import { MessageInput } from "@/components/MessageInput";
import { FileUpload } from "@/components/FileUpload";
import { DocumentList, Document } from "@/components/DocumentList";

export default function RagDemo() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [documents, setDocuments] = useState<Document[]>([]);

  const apiUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8000";
  const ingestionBaseUrl = `${apiUrl}/ingestion`;

  const fetchDocuments = useCallback(async () => {
    try {
      const res = await fetch(`${ingestionBaseUrl}/documents`);
      if (res.ok) {
        const data = await res.json();
        setDocuments(data.documents ?? []);
      }
    } catch {
      // Silently fail — documents list is non-critical
    }
  }, [ingestionBaseUrl]);

  useEffect(() => {
    fetchDocuments();
  }, [fetchDocuments]);

  const handleDelete = useCallback(
    async (documentId: string) => {
      const res = await fetch(`${ingestionBaseUrl}/documents/${documentId}`, {
        method: "DELETE",
      });
      if (res.ok) {
        await fetchDocuments();
      }
    },
    [ingestionBaseUrl, fetchDocuments]
  );

  const handleSend = useCallback(
    async (question: string) => {
      setMessages((prev) => [...prev, { role: "user", content: question }]);
      setMessages((prev) => [...prev, { role: "assistant", content: "" }]);
      setIsStreaming(true);

      try {
        const chatBaseUrl = `${apiUrl}/chat`;
        const res = await fetch(`${chatBaseUrl}/chat`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ question }),
        });

        if (!res.ok) {
          throw new Error("Failed to connect to chat service");
        }

        const reader = res.body?.getReader();
        if (!reader) throw new Error("No response stream");

        const decoder = new TextDecoder();
        let buffer = "";
        let sources: Source[] = [];

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() || "";

          for (const line of lines) {
            if (!line.startsWith("data: ")) continue;
            const jsonStr = line.slice(6).trim();
            if (!jsonStr) continue;

            try {
              const event = JSON.parse(jsonStr);

              if (event.token) {
                setMessages((prev) => {
                  const updated = [...prev];
                  const last = updated[updated.length - 1];
                  updated[updated.length - 1] = {
                    ...last,
                    content: last.content + event.token,
                  };
                  return updated;
                });
              }

              if (event.done && event.sources) {
                sources = event.sources;
              }
            } catch {
              // skip malformed SSE lines
            }
          }
        }

        if (sources.length > 0) {
          setMessages((prev) => {
            const updated = [...prev];
            const last = updated[updated.length - 1];
            updated[updated.length - 1] = { ...last, sources };
            return updated;
          });
        }

        setMessages((prev) => {
          const last = prev[prev.length - 1];
          if (last.role === "assistant" && !last.content) {
            const updated = [...prev];
            updated[updated.length - 1] = {
              ...last,
              content: "No response received.",
            };
            return updated;
          }
          return prev;
        });
      } catch (err) {
        setMessages((prev) => {
          const updated = [...prev];
          updated[updated.length - 1] = {
            role: "assistant",
            content:
              err instanceof Error
                ? err.message
                : "Could not connect to the backend. Make sure the services are running.",
          };
          return updated;
        });
      } finally {
        setIsStreaming(false);
      }
    },
    []
  );

  const handleUploaded = useCallback(
    (_filename: string, _chunks: number) => {
      fetchDocuments();
    },
    [fetchDocuments]
  );

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      {/* Header */}
      <header className="flex items-center justify-between border-b px-6 py-3">
        <div className="flex items-center gap-4">
          <Link
            href="/ai"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            &larr; Back
          </Link>
          <h1 className="text-lg font-semibold">Document Q&A Assistant</h1>
        </div>
        <div className="flex items-center gap-4">
          {documents?.length > 0 && (
            <DocumentList documents={documents} onDelete={handleDelete} />
          )}
          <FileUpload onUploaded={handleUploaded} />
        </div>
      </header>

      {/* Chat */}
      <ChatWindow messages={messages} />

      {/* Input */}
      <MessageInput onSend={handleSend} disabled={isStreaming} />
    </div>
  );
}
