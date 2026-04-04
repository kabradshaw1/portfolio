import Link from "next/link";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";

export default function Home() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-3xl px-6 py-16">
        {/* Name & Bio */}
        <h1 className="text-4xl font-bold">Kyle Bradshaw</h1>
        <p className="mt-6 text-lg text-muted-foreground leading-relaxed">
          [Placeholder: General bio. Introduce yourself, your background in
          software engineering, and what you bring to the table. This page serves
          as the entry point to role-specific sections below.]
        </p>

        {/* Sections */}
        <h2 className="mt-16 text-2xl font-semibold">Portfolio</h2>
        <div className="mt-6 grid gap-4">
          <Link href="/ai" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>AI / Gen AI Engineer</CardTitle>
                <CardDescription>
                  Document Q&A Assistant built with RAG, FastAPI, Qdrant, and
                  Ollama
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  A full-stack retrieval-augmented generation system
                  demonstrating PDF ingestion, vector search, prompt
                  engineering, and streaming LLM responses.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/java" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Full Stack Java Developer</CardTitle>
                <CardDescription>
                  Task Management System built with Spring Boot, GraphQL, and
                  Kubernetes
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Microservices architecture with PostgreSQL, MongoDB, Redis,
                  RabbitMQ, Google OAuth, and CI/CD automation.
                </p>
              </CardContent>
            </Card>
          </Link>
        </div>
      </div>
    </div>
  );
}
