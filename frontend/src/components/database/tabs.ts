export type DatabaseTab = "postgres" | "nosql" | "vector";

export const databaseTabs: { key: DatabaseTab; label: string }[] = [
  { key: "postgres", label: "PostgreSQL" },
  { key: "nosql", label: "NoSQL" },
  { key: "vector", label: "Vector" },
];
