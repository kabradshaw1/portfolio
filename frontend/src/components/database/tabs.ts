export type DatabaseTab = "postgres" | "redis" | "mongodb" | "vector";

export const databaseTabs: { key: DatabaseTab; label: string }[] = [
  { key: "postgres", label: "PostgreSQL" },
  { key: "redis", label: "Redis" },
  { key: "mongodb", label: "MongoDB" },
  { key: "vector", label: "Vector" },
];
