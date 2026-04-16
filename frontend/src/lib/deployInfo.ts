const REPO = "kabradshaw1/gen_ai_engineer";

export interface Commit {
  sha: string;
  message: string;
  date: string;
  url: string;
}

export interface DeployInfo {
  branch: string;
  commitSha: string;
  fullSha: string;
  commitMessage: string;
  commitDate: string;
  isQA: boolean;
  qaAheadOfMain: Commit[];
}

function timeAgo(dateStr: string): string {
  if (!dateStr) return "";
  const seconds = Math.floor(
    (Date.now() - new Date(dateStr).getTime()) / 1000
  );
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

async function fetchCommitDate(sha: string): Promise<string> {
  try {
    const res = await fetch(
      `https://api.github.com/repos/${REPO}/commits/${sha}`,
      { next: { revalidate: false } }
    );
    if (!res.ok) return "";
    const data = await res.json();
    return data.commit?.author?.date ?? "";
  } catch {
    return "";
  }
}

async function fetchQADiff(): Promise<Commit[]> {
  try {
    const res = await fetch(
      `https://api.github.com/repos/${REPO}/compare/main...qa`,
      { next: { revalidate: false } }
    );
    if (!res.ok) return [];
    const data = await res.json();
    const commits: Commit[] = (data.commits ?? [])
      .slice(0, 20)
      .map(
        (c: {
          sha: string;
          commit: { message: string; author: { date: string } };
          html_url: string;
        }) => ({
          sha: c.sha.slice(0, 7),
          message: c.commit.message.split("\n")[0],
          date: c.commit.author.date,
          url: c.html_url,
        })
      );
    return commits;
  } catch {
    return [];
  }
}

export async function getDeployInfo(): Promise<DeployInfo> {
  const branch = process.env.VERCEL_GIT_COMMIT_REF ?? "local";
  const fullSha = process.env.VERCEL_GIT_COMMIT_SHA ?? "";
  const commitSha = fullSha ? fullSha.slice(0, 7) : "dev";
  const commitMessage = process.env.VERCEL_GIT_COMMIT_MESSAGE ?? "";

  const commitDate = fullSha ? await fetchCommitDate(fullSha) : "";
  const qaAheadOfMain = await fetchQADiff();

  return {
    branch,
    commitSha,
    fullSha,
    commitMessage,
    commitDate,
    isQA: branch === "qa",
    qaAheadOfMain,
  };
}

export { timeAgo };
