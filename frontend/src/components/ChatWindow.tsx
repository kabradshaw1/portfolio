"use client";

import { useEffect, useRef } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Card } from "@/components/ui/card";
import { SourceBadge } from "./SourceBadge";

export interface Source {
  file: string;
  page: number;
}

export interface Message {
  role: "user" | "assistant";
  content: string;
  sources?: Source[];
}

interface ChatWindowProps {
  messages: Message[];
}

export function ChatWindow({ messages }: ChatWindowProps) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  return (
    <ScrollArea className="flex-1 p-4">
      <div className="mx-auto max-w-3xl space-y-4">
        {messages.length === 0 && (
          <div className="mx-auto max-w-md pt-24 text-center text-muted-foreground">
            <h2 className="mb-3 text-lg font-medium text-foreground">
              Document Q&A Assistant
            </h2>
            <p className="mb-4 text-sm leading-relaxed">
              This app uses RAG (Retrieval-Augmented Generation) to answer
              questions about your documents. Upload a PDF using the button
              above, then ask a question below.
            </p>
            <p className="text-sm leading-relaxed">
              Your question is matched against the document content, and
              relevant passages are sent to the LLM to generate a grounded
              answer. Source citations (filename and page number) appear below
              each response.
            </p>
          </div>
        )}
        {messages.map((msg, i) => (
          <div
            key={i}
            className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}
          >
            <div className={msg.role === "user" ? "max-w-[70%]" : "max-w-[80%]"}>
              <Card
                className={`px-4 py-3 ${
                  msg.role === "user"
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted"
                }`}
              >
                <p className="whitespace-pre-wrap text-sm">{msg.content}</p>
              </Card>
              {msg.sources && msg.sources.length > 0 && (
                <div className="mt-1.5 flex flex-wrap gap-1.5">
                  {msg.sources.map((source, j) => (
                    <SourceBadge
                      key={j}
                      filename={source.file}
                      page={source.page}
                    />
                  ))}
                </div>
              )}
            </div>
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </ScrollArea>
  );
}
