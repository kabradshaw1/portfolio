# Java Task Management Frontend — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Next.js pages and components for the Java task management app — Google OAuth login, project dashboard, Kanban board, task detail with comments/activity, and notification bell — all powered by Apollo Client talking to the gateway-service GraphQL API.

**Architecture:** New pages under `/java/*` in the existing Next.js frontend. Apollo Client handles all GraphQL queries/mutations to the gateway-service at a configurable URL. Auth tokens stored in localStorage, sent as Bearer headers. Components follow existing shadcn/ui + Tailwind patterns.

**Tech Stack:** Next.js 16.2.1 (App Router), React 19, TypeScript, Apollo Client, shadcn/ui, Tailwind CSS v4, lucide-react icons

**Prerequisite:** Java backend (gateway-service on port 8080) must be running for live testing. All components can be built and type-checked without it.

**IMPORTANT:** Next.js 16.2.1 has breaking changes from training data. Before writing any Next.js code, read the relevant guide in `frontend/node_modules/next/dist/docs/` to verify API usage.

---

## File Structure

```
frontend/src/
├── lib/
│   ├── apollo-client.ts          # Apollo Client singleton with auth header
│   └── auth.ts                   # Token storage helpers + Google OAuth config
├── app/
│   ├── java/
│   │   ├── layout.tsx            # Java section layout with SiteHeader
│   │   ├── page.tsx              # /java — portfolio landing page
│   │   └── tasks/
│   │       ├── page.tsx          # /java/tasks — project dashboard
│   │       ├── [projectId]/
│   │       │   ├── page.tsx      # /java/tasks/[projectId] — Kanban board
│   │       │   └── [taskId]/
│   │       │       └── page.tsx  # Task detail with comments + activity
├── components/
│   ├── java/
│   │   ├── SiteHeader.tsx        # Persistent header (GitHub, LinkedIn, Resume)
│   │   ├── GoogleLoginButton.tsx # OAuth trigger
│   │   ├── AuthProvider.tsx      # React context for auth state
│   │   ├── ProjectList.tsx       # Project cards with create/delete
│   │   ├── CreateProjectDialog.tsx # Modal for new project
│   │   ├── KanbanBoard.tsx       # Three-column task board
│   │   ├── TaskCard.tsx          # Task summary card
│   │   ├── CreateTaskDialog.tsx  # Modal for new task
│   │   ├── TaskDetail.tsx        # Full task view
│   │   ├── CommentSection.tsx    # Comments list + add form
│   │   ├── ActivityTimeline.tsx  # Activity event list
│   │   └── NotificationBell.tsx  # Bell icon with unread badge + dropdown
```

---

## Phase 1: Foundation

### Task 1: Install Apollo Client

**Files:**
- Modify: `frontend/package.json`

- [ ] **Step 1: Install dependencies**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend
npm install @apollo/client graphql
```

- [ ] **Step 2: Verify type check still passes**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "feat(frontend): add Apollo Client and graphql dependencies"
```

---

### Task 2: Apollo Client Setup and Auth Helpers

**Files:**
- Create: `frontend/src/lib/apollo-client.ts`
- Create: `frontend/src/lib/auth.ts`

- [ ] **Step 1: Write auth helpers**

Create `frontend/src/lib/auth.ts`:

```typescript
const ACCESS_TOKEN_KEY = "java_access_token";
const REFRESH_TOKEN_KEY = "java_refresh_token";

export function getAccessToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function getRefreshToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

export function setTokens(accessToken: string, refreshToken: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
}

export function clearTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}

export function isLoggedIn(): boolean {
  return getAccessToken() !== null;
}

export const GOOGLE_CLIENT_ID = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID || "";
export const GATEWAY_URL =
  process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";
```

- [ ] **Step 2: Write Apollo Client**

Create `frontend/src/lib/apollo-client.ts`:

```typescript
import { ApolloClient, InMemoryCache, createHttpLink } from "@apollo/client";
import { setContext } from "@apollo/client/link/context";
import { getAccessToken, GATEWAY_URL } from "./auth";

const httpLink = createHttpLink({
  uri: `${GATEWAY_URL}/graphql`,
});

const authLink = setContext((_, { headers }) => {
  const token = getAccessToken();
  return {
    headers: {
      ...headers,
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  };
});

export const apolloClient = new ApolloClient({
  link: authLink.concat(httpLink),
  cache: new InMemoryCache(),
});
```

- [ ] **Step 3: Verify type check**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/apollo-client.ts frontend/src/lib/auth.ts
git commit -m "feat(frontend): add Apollo Client config and auth token helpers"
```

---

### Task 3: AuthProvider Context

**Files:**
- Create: `frontend/src/components/java/AuthProvider.tsx`

- [ ] **Step 1: Write AuthProvider**

Create `frontend/src/components/java/AuthProvider.tsx`:

```typescript
"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { ApolloProvider } from "@apollo/client";
import { apolloClient } from "@/lib/apollo-client";
import {
  clearTokens,
  getAccessToken,
  isLoggedIn as checkIsLoggedIn,
  setTokens,
  GATEWAY_URL,
} from "@/lib/auth";

interface AuthUser {
  userId: string;
  email: string;
  name: string;
  avatarUrl: string | null;
}

interface AuthContextType {
  user: AuthUser | null;
  isLoggedIn: boolean;
  login: (code: string, redirectUri: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType>({
  user: null,
  isLoggedIn: false,
  login: async () => {},
  logout: () => {},
});

export function useAuth() {
  return useContext(AuthContext);
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [isAuthenticated, setIsAuthenticated] = useState(false);

  useEffect(() => {
    if (checkIsLoggedIn()) {
      setIsAuthenticated(true);
      // User info is stored alongside tokens
      const stored = localStorage.getItem("java_user");
      if (stored) {
        setUser(JSON.parse(stored));
      }
    }
  }, []);

  const login = useCallback(async (code: string, redirectUri: string) => {
    const res = await fetch(`${GATEWAY_URL}/api/auth/google`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code, redirectUri }),
    });
    if (!res.ok) throw new Error("Login failed");
    const data = await res.json();
    setTokens(data.accessToken, data.refreshToken);
    const authUser: AuthUser = {
      userId: data.userId,
      email: data.email,
      name: data.name,
      avatarUrl: data.avatarUrl,
    };
    localStorage.setItem("java_user", JSON.stringify(authUser));
    setUser(authUser);
    setIsAuthenticated(true);
  }, []);

  const logout = useCallback(() => {
    clearTokens();
    localStorage.removeItem("java_user");
    setUser(null);
    setIsAuthenticated(false);
    apolloClient.clearStore();
  }, []);

  return (
    <AuthContext.Provider
      value={{ user, isLoggedIn: isAuthenticated, login, logout }}
    >
      <ApolloProvider client={apolloClient}>{children}</ApolloProvider>
    </AuthContext.Provider>
  );
}
```

- [ ] **Step 2: Verify type check**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/java/AuthProvider.tsx
git commit -m "feat(frontend): add AuthProvider with Google OAuth and Apollo integration"
```

---

## Phase 2: Layout and Landing Pages

### Task 4: SiteHeader Component

**Files:**
- Create: `frontend/src/components/java/SiteHeader.tsx`

- [ ] **Step 1: Write SiteHeader**

Create `frontend/src/components/java/SiteHeader.tsx`:

```typescript
"use client";

import Link from "next/link";
import { Github, Linkedin, FileText } from "lucide-react";
import { useAuth } from "./AuthProvider";
import { NotificationBell } from "./NotificationBell";

export function SiteHeader() {
  const { user, isLoggedIn, logout } = useAuth();

  return (
    <header className="border-b border-foreground/10 bg-background">
      <div className="mx-auto flex h-14 max-w-5xl items-center justify-between px-6">
        <Link href="/java" className="text-lg font-semibold">
          Kyle Bradshaw
        </Link>

        <nav className="flex items-center gap-4">
          <a
            href="https://github.com/kabradshaw1"
            target="_blank"
            rel="noopener noreferrer"
            className="text-muted-foreground hover:text-foreground transition-colors"
          >
            <Github className="size-5" />
          </a>
          <a
            href="https://www.linkedin.com/in/kyle-bradshaw-15950988/"
            target="_blank"
            rel="noopener noreferrer"
            className="text-muted-foreground hover:text-foreground transition-colors"
          >
            <Linkedin className="size-5" />
          </a>
          <a
            href="/resume.pdf"
            className="text-muted-foreground hover:text-foreground transition-colors"
          >
            <FileText className="size-5" />
          </a>

          {isLoggedIn && (
            <>
              <NotificationBell />
              <div className="flex items-center gap-2">
                {user?.avatarUrl && (
                  <img
                    src={user.avatarUrl}
                    alt=""
                    className="size-7 rounded-full"
                  />
                )}
                <span className="text-sm text-muted-foreground">
                  {user?.name}
                </span>
                <button
                  onClick={logout}
                  className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                >
                  Sign out
                </button>
              </div>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}
```

NOTE: NotificationBell is imported but will be created in a later task. Create a stub first:

Create `frontend/src/components/java/NotificationBell.tsx` (stub):

```typescript
"use client";

export function NotificationBell() {
  return null; // Implemented in Task 11
}
```

- [ ] **Step 2: Verify type check**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/java/SiteHeader.tsx frontend/src/components/java/NotificationBell.tsx
git commit -m "feat(frontend): add SiteHeader with nav links and auth display"
```

---

### Task 5: Java Section Layout and Landing Page

**Files:**
- Create: `frontend/src/app/java/layout.tsx`
- Create: `frontend/src/app/java/page.tsx`

- [ ] **Step 1: Write Java layout**

Create `frontend/src/app/java/layout.tsx`:

```typescript
import { AuthProvider } from "@/components/java/AuthProvider";
import { SiteHeader } from "@/components/java/SiteHeader";

export default function JavaLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthProvider>
      <SiteHeader />
      <main className="flex-1">{children}</main>
    </AuthProvider>
  );
}
```

- [ ] **Step 2: Write Java landing page**

Create `frontend/src/app/java/page.tsx`:

```typescript
import Link from "next/link";

export default function JavaPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <Link
        href="/"
        className="text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        &larr; Home
      </Link>

      <h1 className="mt-8 text-3xl font-bold">Full Stack Java Developer</h1>

      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          [Placeholder: Java-focused bio. Describe your experience with Spring
          Boot, microservices, and cloud-native development. Highlight relevant
          projects and what excites you about backend engineering at scale.]
        </p>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Task Management System</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          A full-stack project management application demonstrating Spring Boot
          microservices, PostgreSQL, MongoDB, Redis, RabbitMQ, GraphQL, Google
          OAuth, and Kubernetes — all orchestrated with Docker Compose and
          CI/CD via GitHub Actions.
        </p>

        <h3 className="mt-6 text-lg font-medium">Tech Stack</h3>
        <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
          <li>4 Spring Boot microservices (gateway, task, activity, notification)</li>
          <li>GraphQL API gateway with JWT authentication</li>
          <li>PostgreSQL (tasks), MongoDB (activity logs), Redis (notifications)</li>
          <li>RabbitMQ event-driven architecture</li>
          <li>Google OAuth 2.0 login</li>
          <li>Next.js + TypeScript + Apollo Client frontend</li>
          <li>Docker Compose + Minikube Kubernetes manifests</li>
          <li>CI/CD with GitHub Actions, Testcontainers, security scanning</li>
        </ul>
      </section>

      <section className="mt-12">
        <Link
          href="/java/tasks"
          className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          Open Task Manager &rarr;
        </Link>
      </section>
    </div>
  );
}
```

- [ ] **Step 3: Add /java card to home page**

Modify `frontend/src/app/page.tsx` — add a new card in the Portfolio grid after the AI card:

```typescript
<Link href="/java" className="block">
  <Card className="hover:ring-foreground/20 transition-all">
    <CardHeader>
      <CardTitle>Full Stack Java Developer</CardTitle>
      <CardDescription>
        Task Management System built with Spring Boot, GraphQL, and Kubernetes
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
```

- [ ] **Step 4: Verify type check and build**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app/java/ frontend/src/app/page.tsx
git commit -m "feat(frontend): add /java landing page and layout with SiteHeader"
```

---

### Task 6: GoogleLoginButton

**Files:**
- Create: `frontend/src/components/java/GoogleLoginButton.tsx`

- [ ] **Step 1: Write GoogleLoginButton**

Create `frontend/src/components/java/GoogleLoginButton.tsx`:

```typescript
"use client";

import { useCallback } from "react";
import { Button } from "@/components/ui/button";
import { GOOGLE_CLIENT_ID } from "@/lib/auth";

export function GoogleLoginButton() {
  const handleLogin = useCallback(() => {
    const redirectUri = `${window.location.origin}/java/tasks`;
    const params = new URLSearchParams({
      client_id: GOOGLE_CLIENT_ID,
      redirect_uri: redirectUri,
      response_type: "code",
      scope: "openid email profile",
      access_type: "offline",
      prompt: "consent",
    });
    window.location.href = `https://accounts.google.com/o/oauth2/v2/auth?${params}`;
  }, []);

  return (
    <Button onClick={handleLogin} size="lg">
      Sign in with Google
    </Button>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/java/GoogleLoginButton.tsx
git commit -m "feat(frontend): add GoogleLoginButton component"
```

---

## Phase 3: Project Dashboard

### Task 7: ProjectList and CreateProjectDialog

**Files:**
- Create: `frontend/src/components/java/ProjectList.tsx`
- Create: `frontend/src/components/java/CreateProjectDialog.tsx`
- Create: `frontend/src/app/java/tasks/page.tsx`

- [ ] **Step 1: Write ProjectList**

Create `frontend/src/components/java/ProjectList.tsx`:

```typescript
"use client";

import { useQuery, useMutation, gql } from "@apollo/client";
import Link from "next/link";
import { Trash2, FolderOpen } from "lucide-react";
import { useState } from "react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { CreateProjectDialog } from "./CreateProjectDialog";

const MY_PROJECTS = gql`
  query MyProjects {
    myProjects {
      id
      name
      description
      ownerName
      createdAt
    }
  }
`;

const DELETE_PROJECT = gql`
  mutation DeleteProject($id: ID!) {
    deleteProject(id: $id)
  }
`;

export function ProjectList() {
  const { data, loading, refetch } = useQuery(MY_PROJECTS);
  const [deleteProject] = useMutation(DELETE_PROJECT);
  const [showCreate, setShowCreate] = useState(false);

  if (loading) {
    return <p className="text-muted-foreground">Loading projects...</p>;
  }

  const projects = data?.myProjects ?? [];

  return (
    <div>
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-semibold">My Projects</h2>
        <Button onClick={() => setShowCreate(true)}>New Project</Button>
      </div>

      {projects.length === 0 ? (
        <p className="mt-6 text-muted-foreground">
          No projects yet. Create one to get started.
        </p>
      ) : (
        <div className="mt-6 grid gap-4">
          {projects.map(
            (project: {
              id: string;
              name: string;
              description: string | null;
              ownerName: string;
            }) => (
              <Link
                key={project.id}
                href={`/java/tasks/${project.id}`}
                className="block"
              >
                <Card className="hover:ring-foreground/20 transition-all group">
                  <CardHeader className="flex flex-row items-start justify-between">
                    <div className="flex items-start gap-3">
                      <FolderOpen className="mt-0.5 size-5 text-muted-foreground" />
                      <div>
                        <CardTitle>{project.name}</CardTitle>
                        {project.description && (
                          <CardDescription className="mt-1">
                            {project.description}
                          </CardDescription>
                        )}
                      </div>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="opacity-0 group-hover:opacity-100 transition-opacity"
                      onClick={async (e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        await deleteProject({ variables: { id: project.id } });
                        refetch();
                      }}
                    >
                      <Trash2 className="size-4 text-destructive" />
                    </Button>
                  </CardHeader>
                </Card>
              </Link>
            )
          )}
        </div>
      )}

      {showCreate && (
        <CreateProjectDialog
          onClose={() => setShowCreate(false)}
          onCreated={() => {
            setShowCreate(false);
            refetch();
          }}
        />
      )}
    </div>
  );
}
```

- [ ] **Step 2: Write CreateProjectDialog**

Create `frontend/src/components/java/CreateProjectDialog.tsx`:

```typescript
"use client";

import { useMutation, gql } from "@apollo/client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const CREATE_PROJECT = gql`
  mutation CreateProject($input: CreateProjectInput!) {
    createProject(input: $input) {
      id
      name
    }
  }
`;

interface Props {
  onClose: () => void;
  onCreated: () => void;
}

export function CreateProjectDialog({ onClose, onCreated }: Props) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [createProject, { loading }] = useMutation(CREATE_PROJECT);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    await createProject({
      variables: { input: { name: name.trim(), description: description.trim() || null } },
    });
    onCreated();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-md rounded-xl border border-foreground/10 bg-background p-6">
        <h3 className="text-lg font-semibold">New Project</h3>
        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          <div>
            <label className="text-sm font-medium">Name</label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My Project"
              autoFocus
            />
          </div>
          <div>
            <label className="text-sm font-medium">Description</label>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description"
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={loading || !name.trim()}>
              {loading ? "Creating..." : "Create"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Write /java/tasks page**

Create `frontend/src/app/java/tasks/page.tsx`:

```typescript
"use client";

import { useEffect } from "react";
import { useAuth } from "@/components/java/AuthProvider";
import { GoogleLoginButton } from "@/components/java/GoogleLoginButton";
import { ProjectList } from "@/components/java/ProjectList";
import { useSearchParams } from "next/navigation";

export default function TasksPage() {
  const { isLoggedIn, login } = useAuth();
  const searchParams = useSearchParams();

  // Handle OAuth callback
  useEffect(() => {
    const code = searchParams.get("code");
    if (code && !isLoggedIn) {
      const redirectUri = `${window.location.origin}/java/tasks`;
      login(code, redirectUri).then(() => {
        // Remove code from URL
        window.history.replaceState({}, "", "/java/tasks");
      });
    }
  }, [searchParams, isLoggedIn, login]);

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      {!isLoggedIn ? (
        <div className="flex flex-col items-center gap-6 py-24">
          <h1 className="text-2xl font-semibold">Task Manager</h1>
          <p className="text-muted-foreground">
            Sign in to manage your projects and tasks.
          </p>
          <GoogleLoginButton />
        </div>
      ) : (
        <ProjectList />
      )}
    </div>
  );
}
```

- [ ] **Step 4: Verify type check**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/java/ProjectList.tsx \
        frontend/src/components/java/CreateProjectDialog.tsx \
        frontend/src/app/java/tasks/page.tsx
git commit -m "feat(frontend): add project dashboard with create/delete"
```

---

## Phase 4: Kanban Board

### Task 8: TaskCard and KanbanBoard

**Files:**
- Create: `frontend/src/components/java/TaskCard.tsx`
- Create: `frontend/src/components/java/KanbanBoard.tsx`
- Create: `frontend/src/components/java/CreateTaskDialog.tsx`
- Create: `frontend/src/app/java/tasks/[projectId]/page.tsx`

- [ ] **Step 1: Write TaskCard**

Create `frontend/src/components/java/TaskCard.tsx`:

```typescript
"use client";

import Link from "next/link";
import { Badge } from "@/components/ui/badge";

interface TaskCardProps {
  task: {
    id: string;
    projectId: string;
    title: string;
    priority: string;
    assigneeName: string | null;
    assigneeId: string | null;
  };
}

const priorityColors: Record<string, string> = {
  HIGH: "bg-red-500/10 text-red-500",
  MEDIUM: "bg-yellow-500/10 text-yellow-500",
  LOW: "bg-green-500/10 text-green-500",
};

export function TaskCard({ task }: TaskCardProps) {
  return (
    <Link href={`/java/tasks/${task.projectId}/${task.id}`}>
      <div className="rounded-lg border border-foreground/10 bg-card p-3 hover:ring-1 hover:ring-foreground/20 transition-all cursor-pointer">
        <p className="text-sm font-medium">{task.title}</p>
        <div className="mt-2 flex items-center gap-2">
          <Badge
            variant="secondary"
            className={priorityColors[task.priority] || ""}
          >
            {task.priority}
          </Badge>
          {task.assigneeName && (
            <span className="text-xs text-muted-foreground">
              {task.assigneeName}
            </span>
          )}
        </div>
      </div>
    </Link>
  );
}
```

- [ ] **Step 2: Write CreateTaskDialog**

Create `frontend/src/components/java/CreateTaskDialog.tsx`:

```typescript
"use client";

import { useMutation, gql } from "@apollo/client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const CREATE_TASK = gql`
  mutation CreateTask($input: CreateTaskInput!) {
    createTask(input: $input) {
      id
      title
    }
  }
`;

interface Props {
  projectId: string;
  onClose: () => void;
  onCreated: () => void;
}

export function CreateTaskDialog({ projectId, onClose, onCreated }: Props) {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState("MEDIUM");
  const [createTask, { loading }] = useMutation(CREATE_TASK);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    await createTask({
      variables: {
        input: {
          projectId,
          title: title.trim(),
          description: description.trim() || null,
          priority,
        },
      },
    });
    onCreated();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-md rounded-xl border border-foreground/10 bg-background p-6">
        <h3 className="text-lg font-semibold">New Task</h3>
        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          <div>
            <label className="text-sm font-medium">Title</label>
            <Input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Task title"
              autoFocus
            />
          </div>
          <div>
            <label className="text-sm font-medium">Description</label>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description"
            />
          </div>
          <div>
            <label className="text-sm font-medium">Priority</label>
            <select
              value={priority}
              onChange={(e) => setPriority(e.target.value)}
              className="w-full rounded-lg border border-foreground/10 bg-background px-3 py-2 text-sm"
            >
              <option value="LOW">Low</option>
              <option value="MEDIUM">Medium</option>
              <option value="HIGH">High</option>
            </select>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={loading || !title.trim()}>
              {loading ? "Creating..." : "Create"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Write KanbanBoard**

Create `frontend/src/components/java/KanbanBoard.tsx`:

```typescript
"use client";

import { useQuery, useMutation, gql } from "@apollo/client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { TaskCard } from "./TaskCard";
import { CreateTaskDialog } from "./CreateTaskDialog";

const GET_PROJECT = gql`
  query GetProject($id: ID!) {
    project(id: $id) {
      id
      name
      description
    }
  }
`;

const GET_TASKS = gql`
  query GetTasks($projectId: ID!) {
    # Note: tasks are fetched via the project's task list
    # The gateway resolves this by calling task-service
    myProjects {
      id
    }
  }
`;

// We need a dedicated tasks query — but the gateway schema uses task(id) and myProjects.
// For the kanban board, we'll fetch tasks by making a REST-style query through GraphQL.
// Actually, looking at the schema, there's no "tasks by project" query in GraphQL.
// We'll need to add one or use the task-service REST API directly.
// For now, let's use a direct REST call to the gateway which proxies to task-service.

interface Task {
  id: string;
  projectId: string;
  title: string;
  description: string | null;
  status: string;
  priority: string;
  assigneeId: string | null;
  assigneeName: string | null;
}

const UPDATE_TASK = gql`
  mutation UpdateTask($id: ID!, $input: UpdateTaskInput!) {
    updateTask(id: $id, input: $input) {
      id
      status
    }
  }
`;

interface Props {
  projectId: string;
  tasks: Task[];
  refetch: () => void;
}

const columns = [
  { key: "TODO", label: "To Do" },
  { key: "IN_PROGRESS", label: "In Progress" },
  { key: "DONE", label: "Done" },
] as const;

export function KanbanBoard({ projectId, tasks, refetch }: Props) {
  const [showCreate, setShowCreate] = useState(false);
  const [updateTask] = useMutation(UPDATE_TASK);

  const moveTask = async (taskId: string, newStatus: string) => {
    await updateTask({
      variables: { id: taskId, input: { status: newStatus } },
    });
    refetch();
  };

  return (
    <div>
      <div className="flex items-center justify-between">
        <div />
        <Button onClick={() => setShowCreate(true)}>New Task</Button>
      </div>

      <div className="mt-6 grid grid-cols-3 gap-4">
        {columns.map((col) => {
          const columnTasks = tasks.filter((t) => t.status === col.key);
          return (
            <div
              key={col.key}
              className="rounded-xl border border-foreground/10 bg-card/50 p-4"
            >
              <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
                {col.label}{" "}
                <span className="text-xs">({columnTasks.length})</span>
              </h3>
              <div className="mt-3 space-y-2">
                {columnTasks.map((task) => (
                  <div key={task.id}>
                    <TaskCard task={task} />
                    <div className="mt-1 flex gap-1">
                      {columns
                        .filter((c) => c.key !== task.status)
                        .map((c) => (
                          <button
                            key={c.key}
                            onClick={() => moveTask(task.id, c.key)}
                            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
                          >
                            &rarr; {c.label}
                          </button>
                        ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          );
        })}
      </div>

      {showCreate && (
        <CreateTaskDialog
          projectId={projectId}
          onClose={() => setShowCreate(false)}
          onCreated={() => {
            setShowCreate(false);
            refetch();
          }}
        />
      )}
    </div>
  );
}
```

- [ ] **Step 4: Write project page**

Create `frontend/src/app/java/tasks/[projectId]/page.tsx`:

```typescript
"use client";

import { useParams } from "next/navigation";
import { useQuery, gql } from "@apollo/client";
import Link from "next/link";
import { KanbanBoard } from "@/components/java/KanbanBoard";
import { GATEWAY_URL } from "@/lib/auth";
import { getAccessToken } from "@/lib/auth";
import { useEffect, useState } from "react";

const GET_PROJECT = gql`
  query GetProject($id: ID!) {
    project(id: $id) {
      id
      name
      description
    }
  }
`;

interface Task {
  id: string;
  projectId: string;
  title: string;
  description: string | null;
  status: string;
  priority: string;
  assigneeId: string | null;
  assigneeName: string | null;
}

export default function ProjectPage() {
  const params = useParams();
  const projectId = params.projectId as string;
  const { data: projectData, loading: projectLoading } = useQuery(GET_PROJECT, {
    variables: { id: projectId },
  });
  const [tasks, setTasks] = useState<Task[]>([]);
  const [tasksLoading, setTasksLoading] = useState(true);

  const fetchTasks = async () => {
    setTasksLoading(true);
    const token = getAccessToken();
    const res = await fetch(
      `${GATEWAY_URL}/api/tasks?projectId=${projectId}`,
      {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      }
    );
    if (res.ok) {
      setTasks(await res.json());
    }
    setTasksLoading(false);
  };

  useEffect(() => {
    fetchTasks();
  }, [projectId]);

  if (projectLoading || tasksLoading) {
    return (
      <div className="mx-auto max-w-5xl px-6 py-12">
        <p className="text-muted-foreground">Loading...</p>
      </div>
    );
  }

  const project = projectData?.project;

  return (
    <div className="mx-auto max-w-5xl px-6 py-12">
      <Link
        href="/java/tasks"
        className="text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        &larr; Projects
      </Link>
      <h1 className="mt-4 text-2xl font-bold">{project?.name}</h1>
      {project?.description && (
        <p className="mt-1 text-muted-foreground">{project.description}</p>
      )}
      <div className="mt-8">
        <KanbanBoard
          projectId={projectId}
          tasks={tasks}
          refetch={fetchTasks}
        />
      </div>
    </div>
  );
}
```

**Note:** The GraphQL schema doesn't have a "tasks by project" query, so we fall back to the REST API on the gateway (which proxies to task-service). This is a pragmatic choice — the gateway passes through REST endpoints alongside GraphQL.

- [ ] **Step 5: Verify type check**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/java/TaskCard.tsx \
        frontend/src/components/java/KanbanBoard.tsx \
        frontend/src/components/java/CreateTaskDialog.tsx \
        frontend/src/app/java/tasks/\\[projectId\\]/page.tsx
git commit -m "feat(frontend): add Kanban board with task cards and status transitions"
```

---

## Phase 5: Task Detail

### Task 9: TaskDetail, CommentSection, and ActivityTimeline

**Files:**
- Create: `frontend/src/components/java/TaskDetail.tsx`
- Create: `frontend/src/components/java/CommentSection.tsx`
- Create: `frontend/src/components/java/ActivityTimeline.tsx`
- Create: `frontend/src/app/java/tasks/[projectId]/[taskId]/page.tsx`

- [ ] **Step 1: Write CommentSection**

Create `frontend/src/components/java/CommentSection.tsx`:

```typescript
"use client";

import { useQuery, useMutation, gql } from "@apollo/client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const GET_COMMENTS = gql`
  query TaskComments($taskId: ID!) {
    taskComments(taskId: $taskId) {
      id
      authorId
      body
      createdAt
    }
  }
`;

const ADD_COMMENT = gql`
  mutation AddComment($taskId: ID!, $body: String!) {
    addComment(taskId: $taskId, body: $body) {
      id
      body
    }
  }
`;

export function CommentSection({ taskId }: { taskId: string }) {
  const { data, loading, refetch } = useQuery(GET_COMMENTS, {
    variables: { taskId },
  });
  const [addComment] = useMutation(ADD_COMMENT);
  const [body, setBody] = useState("");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!body.trim()) return;
    await addComment({ variables: { taskId, body: body.trim() } });
    setBody("");
    refetch();
  };

  const comments = data?.taskComments ?? [];

  return (
    <div>
      <h3 className="text-lg font-semibold">Comments</h3>
      <div className="mt-4 space-y-3">
        {loading && <p className="text-sm text-muted-foreground">Loading...</p>}
        {comments.map(
          (c: { id: string; authorId: string; body: string; createdAt: string }) => (
            <div
              key={c.id}
              className="rounded-lg border border-foreground/10 bg-card p-3"
            >
              <p className="text-sm">{c.body}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {new Date(c.createdAt).toLocaleString()}
              </p>
            </div>
          )
        )}
      </div>
      <form onSubmit={handleSubmit} className="mt-4 flex gap-2">
        <Input
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Add a comment..."
          className="flex-1"
        />
        <Button type="submit" disabled={!body.trim()}>
          Post
        </Button>
      </form>
    </div>
  );
}
```

- [ ] **Step 2: Write ActivityTimeline**

Create `frontend/src/components/java/ActivityTimeline.tsx`:

```typescript
"use client";

import { useQuery, gql } from "@apollo/client";

const GET_ACTIVITY = gql`
  query TaskActivity($taskId: ID!) {
    taskActivity(taskId: $taskId) {
      id
      eventType
      actorId
      timestamp
    }
  }
`;

const eventLabels: Record<string, string> = {
  TASK_CREATED: "created this task",
  TASK_ASSIGNED: "assigned this task",
  STATUS_CHANGED: "changed the status",
  COMMENT_ADDED: "added a comment",
  TASK_DELETED: "deleted this task",
};

export function ActivityTimeline({ taskId }: { taskId: string }) {
  const { data, loading } = useQuery(GET_ACTIVITY, {
    variables: { taskId },
  });

  const events = data?.taskActivity ?? [];

  return (
    <div>
      <h3 className="text-lg font-semibold">Activity</h3>
      <div className="mt-4 space-y-2">
        {loading && <p className="text-sm text-muted-foreground">Loading...</p>}
        {events.map(
          (e: {
            id: string;
            eventType: string;
            actorId: string;
            timestamp: string;
          }) => (
            <div key={e.id} className="flex items-center gap-2 text-sm">
              <div className="size-2 rounded-full bg-muted-foreground" />
              <span className="text-muted-foreground">
                {eventLabels[e.eventType] || e.eventType}
              </span>
              <span className="text-xs text-muted-foreground">
                {new Date(e.timestamp).toLocaleString()}
              </span>
            </div>
          )
        )}
        {!loading && events.length === 0 && (
          <p className="text-sm text-muted-foreground">No activity yet.</p>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Write TaskDetail**

Create `frontend/src/components/java/TaskDetail.tsx`:

```typescript
"use client";

import { useQuery, useMutation, gql } from "@apollo/client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CommentSection } from "./CommentSection";
import { ActivityTimeline } from "./ActivityTimeline";

const GET_TASK = gql`
  query GetTask($id: ID!) {
    task(id: $id) {
      id
      projectId
      title
      description
      status
      priority
      assigneeName
      dueDate
      createdAt
      updatedAt
    }
  }
`;

const UPDATE_TASK = gql`
  mutation UpdateTask($id: ID!, $input: UpdateTaskInput!) {
    updateTask(id: $id, input: $input) {
      id
      status
    }
  }
`;

const DELETE_TASK = gql`
  mutation DeleteTask($id: ID!) {
    deleteTask(id: $id)
  }
`;

const statusOptions = ["TODO", "IN_PROGRESS", "DONE"];
const priorityColors: Record<string, string> = {
  HIGH: "bg-red-500/10 text-red-500",
  MEDIUM: "bg-yellow-500/10 text-yellow-500",
  LOW: "bg-green-500/10 text-green-500",
};

interface Props {
  taskId: string;
  onDeleted: () => void;
}

export function TaskDetail({ taskId, onDeleted }: Props) {
  const { data, loading, refetch } = useQuery(GET_TASK, {
    variables: { id: taskId },
  });
  const [updateTask] = useMutation(UPDATE_TASK);
  const [deleteTask] = useMutation(DELETE_TASK);

  if (loading) return <p className="text-muted-foreground">Loading...</p>;

  const task = data?.task;
  if (!task) return <p className="text-muted-foreground">Task not found.</p>;

  const handleStatusChange = async (status: string) => {
    await updateTask({ variables: { id: taskId, input: { status } } });
    refetch();
  };

  const handleDelete = async () => {
    await deleteTask({ variables: { id: taskId } });
    onDeleted();
  };

  return (
    <div className="space-y-8">
      <div>
        <div className="flex items-start justify-between">
          <h1 className="text-2xl font-bold">{task.title}</h1>
          <Button variant="destructive" size="sm" onClick={handleDelete}>
            Delete
          </Button>
        </div>
        {task.description && (
          <p className="mt-2 text-muted-foreground">{task.description}</p>
        )}
      </div>

      <div className="flex flex-wrap gap-3">
        <Badge
          variant="secondary"
          className={priorityColors[task.priority] || ""}
        >
          {task.priority}
        </Badge>
        {task.assigneeName && (
          <Badge variant="outline">Assigned: {task.assigneeName}</Badge>
        )}
        {task.dueDate && (
          <Badge variant="outline">Due: {task.dueDate}</Badge>
        )}
      </div>

      <div>
        <h3 className="text-sm font-medium text-muted-foreground">Status</h3>
        <div className="mt-2 flex gap-2">
          {statusOptions.map((s) => (
            <Button
              key={s}
              variant={task.status === s ? "default" : "outline"}
              size="sm"
              onClick={() => handleStatusChange(s)}
            >
              {s.replace("_", " ")}
            </Button>
          ))}
        </div>
      </div>

      <CommentSection taskId={taskId} />
      <ActivityTimeline taskId={taskId} />
    </div>
  );
}
```

- [ ] **Step 4: Write task detail page**

Create `frontend/src/app/java/tasks/[projectId]/[taskId]/page.tsx`:

```typescript
"use client";

import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { TaskDetail } from "@/components/java/TaskDetail";

export default function TaskPage() {
  const params = useParams();
  const router = useRouter();
  const projectId = params.projectId as string;
  const taskId = params.taskId as string;

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <Link
        href={`/java/tasks/${projectId}`}
        className="text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        &larr; Back to board
      </Link>
      <div className="mt-6">
        <TaskDetail
          taskId={taskId}
          onDeleted={() => router.push(`/java/tasks/${projectId}`)}
        />
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Verify type check**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/java/TaskDetail.tsx \
        frontend/src/components/java/CommentSection.tsx \
        frontend/src/components/java/ActivityTimeline.tsx \
        frontend/src/app/java/tasks/\\[projectId\\]/\\[taskId\\]/page.tsx
git commit -m "feat(frontend): add task detail page with comments and activity timeline"
```

---

## Phase 6: Notifications

### Task 10: NotificationBell (replace stub)

**Files:**
- Modify: `frontend/src/components/java/NotificationBell.tsx`

- [ ] **Step 1: Write full NotificationBell**

Replace `frontend/src/components/java/NotificationBell.tsx`:

```typescript
"use client";

import { useQuery, useMutation, gql } from "@apollo/client";
import { Bell } from "lucide-react";
import { useState, useRef, useEffect } from "react";

const GET_NOTIFICATIONS = gql`
  query MyNotifications($unreadOnly: Boolean) {
    myNotifications(unreadOnly: $unreadOnly) {
      notifications {
        id
        type
        message
        taskId
        read
        createdAt
      }
      unreadCount
    }
  }
`;

const MARK_READ = gql`
  mutation MarkNotificationRead($id: ID!) {
    markNotificationRead(id: $id)
  }
`;

const MARK_ALL_READ = gql`
  mutation MarkAllNotificationsRead {
    markAllNotificationsRead
  }
`;

export function NotificationBell() {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const { data, refetch } = useQuery(GET_NOTIFICATIONS, {
    variables: { unreadOnly: false },
    pollInterval: 30000,
  });
  const [markRead] = useMutation(MARK_READ);
  const [markAllRead] = useMutation(MARK_ALL_READ);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const unreadCount = data?.myNotifications?.unreadCount ?? 0;
  const notifications = data?.myNotifications?.notifications ?? [];

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(!open)}
        className="relative text-muted-foreground hover:text-foreground transition-colors"
      >
        <Bell className="size-5" />
        {unreadCount > 0 && (
          <span className="absolute -top-1 -right-1 flex size-4 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-primary-foreground">
            {unreadCount > 9 ? "9+" : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-8 z-50 w-80 rounded-xl border border-foreground/10 bg-background shadow-lg">
          <div className="flex items-center justify-between border-b border-foreground/10 px-4 py-2">
            <span className="text-sm font-medium">Notifications</span>
            {unreadCount > 0 && (
              <button
                onClick={async () => {
                  await markAllRead();
                  refetch();
                }}
                className="text-xs text-primary hover:underline"
              >
                Mark all read
              </button>
            )}
          </div>
          <div className="max-h-64 overflow-y-auto">
            {notifications.length === 0 ? (
              <p className="px-4 py-6 text-sm text-muted-foreground text-center">
                No notifications
              </p>
            ) : (
              notifications.map(
                (n: {
                  id: string;
                  message: string;
                  read: boolean;
                  createdAt: string;
                }) => (
                  <div
                    key={n.id}
                    className={`px-4 py-3 border-b border-foreground/5 cursor-pointer hover:bg-muted/50 ${
                      !n.read ? "bg-primary/5" : ""
                    }`}
                    onClick={async () => {
                      if (!n.read) {
                        await markRead({ variables: { id: n.id } });
                        refetch();
                      }
                    }}
                  >
                    <p className="text-sm">{n.message}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {new Date(n.createdAt).toLocaleString()}
                    </p>
                  </div>
                )
              )
            )}
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify type check**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/java/NotificationBell.tsx
git commit -m "feat(frontend): implement NotificationBell with unread badge and dropdown"
```

---

## Phase 7: Final Verification

### Task 11: Type Check and Build

- [ ] **Step 1: Run full type check**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit
```

Expected: No errors.

- [ ] **Step 2: Run build**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npm run build
```

Expected: Build succeeds.

- [ ] **Step 3: Fix any issues and commit**

```bash
git add frontend/
git commit -m "chore(frontend): fix any build issues"
```

(Skip if no changes needed.)

---

## Summary

**11 tasks** across 7 phases covering:
- Apollo Client setup with auth headers
- AuthProvider context with Google OAuth flow
- SiteHeader with nav links and user display
- /java landing page + /java/tasks project dashboard
- ProjectList with create/delete
- KanbanBoard with status transitions
- TaskDetail with status controls, comments, activity timeline
- NotificationBell with unread badge and dropdown

**Environment variables needed:**
- `NEXT_PUBLIC_GATEWAY_URL` — gateway-service URL (default: http://localhost:8080)
- `NEXT_PUBLIC_GOOGLE_CLIENT_ID` — Google OAuth client ID
