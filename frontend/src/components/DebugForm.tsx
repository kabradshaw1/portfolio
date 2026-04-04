"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";

interface DebugFormData {
  collection: string;
  description: string;
  errorOutput?: string;
}

interface DebugFormProps {
  onSubmit: (data: DebugFormData) => void;
  isLoading: boolean;
}

type IndexStatus = "idle" | "indexing" | "indexed" | "error";

export function DebugForm({ onSubmit, isLoading }: DebugFormProps) {
  const [projectPath, setProjectPath] = useState("");
  const [collection, setCollection] = useState("");
  const [description, setDescription] = useState("");
  const [errorOutput, setErrorOutput] = useState("");
  const [indexStatus, setIndexStatus] = useState<IndexStatus>("idle");
  const [indexMessage, setIndexMessage] = useState<string | null>(null);

  const apiUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8000";
  const debugApiUrl = `${apiUrl}/debug`;

  const handleIndex = async () => {
    if (!projectPath.trim()) return;

    setIndexStatus("indexing");
    setIndexMessage("Indexing project...");

    try {
      const res = await fetch(`${debugApiUrl}/index`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: projectPath.trim() }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ detail: "Indexing failed" }));
        throw new Error(err.detail || "Indexing failed");
      }

      const data = await res.json();
      const col: string = data.collection ?? projectPath.trim();
      setCollection(col);
      setIndexStatus("indexed");
      setIndexMessage(
        `Indexed ${data.chunks_created ?? "?"} chunks into collection "${col}"`
      );
    } catch (err) {
      setIndexStatus("error");
      setIndexMessage(
        err instanceof Error ? err.message : "Failed to index project"
      );
    }
  };

  const canDebug = indexStatus === "indexed" && description.trim().length > 0;

  const handleSubmit = () => {
    if (!canDebug || isLoading) return;
    onSubmit({
      collection,
      description: description.trim(),
      errorOutput: errorOutput.trim() || undefined,
    });
  };

  return (
    <div className="flex flex-col gap-6">
      {/* Project Path + Index */}
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">Project Path</label>
        <div className="flex gap-2">
          <Input
            value={projectPath}
            onChange={(e) => {
              setProjectPath(e.target.value);
              if (indexStatus !== "idle") {
                setIndexStatus("idle");
                setIndexMessage(null);
                setCollection("");
              }
            }}
            placeholder="/path/to/your/project"
            disabled={indexStatus === "indexing"}
            className="flex-1"
          />
          <Button
            variant="secondary"
            onClick={handleIndex}
            disabled={!projectPath.trim() || indexStatus === "indexing"}
          >
            {indexStatus === "indexing" ? "Indexing..." : "Index"}
          </Button>
        </div>
        {indexMessage && (
          <p
            className={`text-xs ${
              indexStatus === "error"
                ? "text-destructive"
                : indexStatus === "indexed"
                  ? "text-green-500"
                  : "text-muted-foreground"
            }`}
          >
            {indexMessage}
          </p>
        )}
      </div>

      {/* Bug Description */}
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">Bug Description</label>
        <Textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Describe the bug or unexpected behavior you're seeing..."
          disabled={isLoading}
          className="min-h-24 resize-none"
        />
      </div>

      {/* Error Output (optional) */}
      <div className="flex flex-col gap-2">
        <label className="text-sm font-medium">
          Error Output{" "}
          <span className="text-muted-foreground font-normal">(optional)</span>
        </label>
        <Textarea
          value={errorOutput}
          onChange={(e) => setErrorOutput(e.target.value)}
          placeholder="Paste stack traces, error messages, or logs here..."
          disabled={isLoading}
          className="min-h-24 resize-none font-mono text-xs"
        />
      </div>

      {/* Debug Button */}
      <Button
        onClick={handleSubmit}
        disabled={!canDebug || isLoading}
        className="w-full"
      >
        {isLoading ? "Debugging..." : "Debug"}
      </Button>
    </div>
  );
}
